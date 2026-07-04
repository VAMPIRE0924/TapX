#include "tapx_fastpath.h"

#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/eventfd.h>
#include <unistd.h>

#define TAPX_DEFAULT_MAX_FRAME_SIZE 65535U
#define TAPX_EPOLL_MAX_EVENTS 3
#define TAPX_ETH_HEADER_LEN 14U
#define TAPX_ETHERTYPE_IPV4 0x0800U
#define TAPX_ETHERTYPE_ARP 0x0806U
#define TAPX_ETHERTYPE_IPV6 0x86DDU
#define TAPX_ARP_ETH_IPV4_LEN 28U
#define TAPX_IPV6_HEADER_LEN 40U
#define TAPX_IPV6_NEXT_ICMPV6 58U
#define TAPX_ICMPV6_NS 135U
#define TAPX_ICMPV6_NA 136U
#define TAPX_ND_OPT_SOURCE_LLADDR 1U
#define TAPX_ND_OPT_TARGET_LLADDR 2U
#define TAPX_VKEY_HEADER_BASE_SIZE 8U
#define TAPX_VKEY_MAX_LEN 1024U
#define TAPX_VKEY_MAGIC_0 'T'
#define TAPX_VKEY_MAGIC_1 'X'
#define TAPX_VKEY_MAGIC_2 'V'
#define TAPX_VKEY_MAGIC_3 '1'

struct tapx_worker {
    pthread_t thread;
    int epoll_fd;
    int stop_fd;
    int tun_fd;
    int udp_fd;
    int tcp_fd;
    uint32_t frame_kind;
    uint32_t max_frame_size;
    uint32_t peer_mode;
    uint32_t length_mode;
    uint32_t header_size;
    uint32_t vkey_header_size;
    uint8_t *buffer;
    uint8_t *stream_buffer;
    uint8_t *vkey_value;
    size_t vkey_len;
    size_t stream_len;
    size_t stream_cap;
    struct sockaddr_storage peer_addr;
    socklen_t peer_addr_len;
    struct tapx_ipv4_prefix *ipv4_prefixes;
    size_t ipv4_prefix_count;
    struct tapx_ipv6_prefix *ipv6_prefixes;
    size_t ipv6_prefix_count;
    struct tapx_mac_addr *macs;
    size_t mac_count;
    int has_peer;
    struct tapx_fastpath_counters *counters;
};

static int tapx_set_nonblock(int fd) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags < 0) {
        return -errno;
    }
    if ((flags & O_NONBLOCK) != 0) {
        return 0;
    }
    if (fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0) {
        return -errno;
    }
    return 0;
}

static void tapx_count_rx(struct tapx_worker *worker, ssize_t n) {
    if (worker->counters == NULL || n <= 0) {
        return;
    }
    worker->counters->rx_packets++;
    worker->counters->rx_bytes += (uint64_t)n;
}

static void tapx_count_tx(struct tapx_worker *worker, ssize_t n) {
    if (worker->counters == NULL || n <= 0) {
        return;
    }
    worker->counters->tx_packets++;
    worker->counters->tx_bytes += (uint64_t)n;
}

static void tapx_count_io_drop(struct tapx_worker *worker) {
    if (worker->counters == NULL) {
        return;
    }
    worker->counters->drops_io++;
}

static void tapx_count_guard_drop(struct tapx_worker *worker) {
    if (worker->counters == NULL) {
        return;
    }
    worker->counters->drops_guard++;
}

static int tapx_epoll_add(int epoll_fd, int fd, uint32_t events) {
    struct epoll_event event;
    memset(&event, 0, sizeof(event));
    event.events = events;
    event.data.fd = fd;
    if (epoll_ctl(epoll_fd, EPOLL_CTL_ADD, fd, &event) < 0) {
        return -errno;
    }
    return 0;
}

static int tapx_peer_equal(const struct sockaddr_storage *a, socklen_t a_len,
                           const struct sockaddr_storage *b, socklen_t b_len) {
    if (a_len != b_len) {
        return 0;
    }
    return memcmp(a, b, a_len) == 0;
}

static uint16_t tapx_read_be16(const uint8_t *p) {
    return (uint16_t)(((uint16_t)p[0] << 8) | (uint16_t)p[1]);
}

static uint32_t tapx_read_be32(const uint8_t *p) {
    return ((uint32_t)p[0] << 24) |
           ((uint32_t)p[1] << 16) |
           ((uint32_t)p[2] << 8) |
           (uint32_t)p[3];
}

static void tapx_write_be16(uint8_t *p, uint16_t value) {
    p[0] = (uint8_t)(value >> 8);
    p[1] = (uint8_t)(value & 0xff);
}

static void tapx_write_be32(uint8_t *p, uint32_t value) {
    p[0] = (uint8_t)(value >> 24);
    p[1] = (uint8_t)((value >> 16) & 0xff);
    p[2] = (uint8_t)((value >> 8) & 0xff);
    p[3] = (uint8_t)(value & 0xff);
}

static int tapx_vkey_enabled(const struct tapx_worker *worker) {
    return worker->vkey_len > 0;
}

static void tapx_write_vkey_header(const struct tapx_worker *worker, uint8_t *p) {
    if (!tapx_vkey_enabled(worker)) {
        return;
    }
    p[0] = TAPX_VKEY_MAGIC_0;
    p[1] = TAPX_VKEY_MAGIC_1;
    p[2] = TAPX_VKEY_MAGIC_2;
    p[3] = TAPX_VKEY_MAGIC_3;
    tapx_write_be16(p + 4, (uint16_t)worker->vkey_len);
    p[6] = 0;
    p[7] = 0;
    memcpy(p + TAPX_VKEY_HEADER_BASE_SIZE, worker->vkey_value, worker->vkey_len);
}

static int tapx_strip_vkey_header(struct tapx_worker *worker, const uint8_t **payload,
                                  size_t *len) {
    if (!tapx_vkey_enabled(worker)) {
        return 1;
    }
    if (*len < worker->vkey_header_size) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    const uint8_t *p = *payload;
    if (p[0] != TAPX_VKEY_MAGIC_0 || p[1] != TAPX_VKEY_MAGIC_1 ||
        p[2] != TAPX_VKEY_MAGIC_2 || p[3] != TAPX_VKEY_MAGIC_3) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    uint16_t key_len = tapx_read_be16(p + 4);
    if (key_len != worker->vkey_len || p[6] != 0 || p[7] != 0) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    if (memcmp(p + TAPX_VKEY_HEADER_BASE_SIZE, worker->vkey_value, worker->vkey_len) != 0) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    *payload = p + worker->vkey_header_size;
    *len -= worker->vkey_header_size;
    return 1;
}

static int tapx_ipv4_prefix_match(const struct tapx_worker *worker, uint32_t addr) {
    for (size_t i = 0; i < worker->ipv4_prefix_count; i++) {
        const struct tapx_ipv4_prefix *prefix = &worker->ipv4_prefixes[i];
        if ((addr & prefix->mask) == prefix->network) {
            return 1;
        }
    }
    return 0;
}

static int tapx_ipv6_prefix_match(const struct tapx_worker *worker, const uint8_t *addr) {
    for (size_t i = 0; i < worker->ipv6_prefix_count; i++) {
        const struct tapx_ipv6_prefix *prefix = &worker->ipv6_prefixes[i];
        int matched = 1;
        for (size_t j = 0; j < 16; j++) {
            if ((addr[j] & prefix->mask[j]) != prefix->network[j]) {
                matched = 0;
                break;
            }
        }
        if (matched) {
            return 1;
        }
    }
    return 0;
}

static int tapx_mac_match(const struct tapx_worker *worker, const uint8_t *mac) {
    for (size_t i = 0; i < worker->mac_count; i++) {
        if (memcmp(worker->macs[i].bytes, mac, 6) == 0) {
            return 1;
        }
    }
    return 0;
}

static int tapx_mac_is_zero(const uint8_t *mac) {
    static const uint8_t zero[6] = {0};
    return memcmp(mac, zero, 6) == 0;
}

static int tapx_mac_is_multicast_or_broadcast(const uint8_t *mac) {
    return (mac[0] & 0x01U) != 0;
}

static int tapx_ipv6_is_unspecified(const uint8_t *addr) {
    static const uint8_t zero[16] = {0};
    return memcmp(addr, zero, 16) == 0;
}

static int tapx_ipv6_is_multicast(const uint8_t *addr) {
    return addr[0] == 0xffU;
}

static int tapx_tun_ip_guard_allows(struct tapx_worker *worker, const uint8_t *packet,
                                    size_t len, int source_address) {
    if (worker->ipv4_prefix_count == 0) {
        if (worker->ipv6_prefix_count == 0) {
            return 1;
        }
    }
    if (len == 0) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    uint8_t version = packet[0] >> 4;
    if (version == 4) {
        if (worker->ipv4_prefix_count == 0) {
            return 1;
        }
        if (len < 20) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        uint32_t addr = tapx_read_be32(packet + (source_address ? 12 : 16));
        if (!tapx_ipv4_prefix_match(worker, addr)) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        return 1;
    }
    if (version == 6) {
        if (worker->ipv6_prefix_count == 0) {
            return 1;
        }
        if (len < TAPX_IPV6_HEADER_LEN) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        const uint8_t *addr = packet + (source_address ? 8 : 24);
        if (!tapx_ipv6_prefix_match(worker, addr)) {
            tapx_count_guard_drop(worker);
            return 0;
        }
    }
    return 1;
}

static int tapx_tap_ipv4_guard_allows(struct tapx_worker *worker, const uint8_t *packet,
                                      size_t len, int source_address) {
    if (worker->ipv4_prefix_count == 0) {
        return 1;
    }
    if (len < 20) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    if ((packet[0] >> 4) != 4) {
        return 1;
    }
    uint32_t addr = tapx_read_be32(packet + (source_address ? 12 : 16));
    if (!tapx_ipv4_prefix_match(worker, addr)) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    return 1;
}

static const uint8_t *tapx_nd_target(const uint8_t *packet, size_t len) {
    if (len < TAPX_IPV6_HEADER_LEN + 24U) {
        return NULL;
    }
    if ((packet[0] >> 4) != 6 || packet[6] != TAPX_IPV6_NEXT_ICMPV6) {
        return NULL;
    }
    const uint8_t *icmp = packet + TAPX_IPV6_HEADER_LEN;
    if (icmp[0] != TAPX_ICMPV6_NS && icmp[0] != TAPX_ICMPV6_NA) {
        return NULL;
    }
    return icmp + 8;
}

static int tapx_nd_option_mac_allows(struct tapx_worker *worker, const uint8_t *packet,
                                     size_t len, uint8_t option_type) {
    if (worker->mac_count == 0) {
        return 1;
    }
    if (len < TAPX_IPV6_HEADER_LEN + 24U) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    size_t offset = TAPX_IPV6_HEADER_LEN + 24U;
    while (offset + 2U <= len) {
        uint8_t kind = packet[offset];
        uint8_t units = packet[offset + 1];
        if (units == 0) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        size_t option_len = (size_t)units * 8U;
        if (option_len < 2U || offset + option_len > len) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        if (kind == option_type) {
            if (option_len < 8U || !tapx_mac_match(worker, packet + offset + 2U)) {
                tapx_count_guard_drop(worker);
                return 0;
            }
            return 1;
        }
        offset += option_len;
    }
    return 1;
}

static int tapx_tap_nd_guard_allows(struct tapx_worker *worker, const uint8_t *packet,
                                    size_t len, int source_address) {
    if (len < TAPX_IPV6_HEADER_LEN + 24U) {
        return 1;
    }
    if (packet[6] != TAPX_IPV6_NEXT_ICMPV6) {
        return 1;
    }
    const uint8_t *icmp = packet + TAPX_IPV6_HEADER_LEN;
    if (icmp[0] == TAPX_ICMPV6_NS) {
        if (source_address) {
            if (!tapx_nd_option_mac_allows(worker, packet, len, TAPX_ND_OPT_SOURCE_LLADDR)) {
                return 0;
            }
            const uint8_t *src = packet + 8;
            const uint8_t *target = icmp + 8;
            if (worker->ipv6_prefix_count > 0 && tapx_ipv6_is_unspecified(src) &&
                !tapx_ipv6_prefix_match(worker, target)) {
                tapx_count_guard_drop(worker);
                return 0;
            }
        } else if (worker->ipv6_prefix_count > 0 && !tapx_ipv6_prefix_match(worker, icmp + 8)) {
            tapx_count_guard_drop(worker);
            return 0;
        }
    } else if (icmp[0] == TAPX_ICMPV6_NA) {
        if (source_address) {
            if (worker->ipv6_prefix_count > 0 && !tapx_ipv6_prefix_match(worker, icmp + 8)) {
                tapx_count_guard_drop(worker);
                return 0;
            }
            if (!tapx_nd_option_mac_allows(worker, packet, len, TAPX_ND_OPT_TARGET_LLADDR)) {
                return 0;
            }
        }
    }
    return 1;
}

static int tapx_tap_ipv6_guard_allows(struct tapx_worker *worker, const uint8_t *packet,
                                      size_t len, int source_address) {
    if (worker->ipv6_prefix_count == 0 && worker->mac_count == 0) {
        return 1;
    }
    if (len < TAPX_IPV6_HEADER_LEN || (packet[0] >> 4) != 6) {
        tapx_count_guard_drop(worker);
        return 0;
    }

    if (worker->ipv6_prefix_count > 0) {
        const uint8_t *addr = packet + (source_address ? 8 : 24);
        int addr_allowed = tapx_ipv6_prefix_match(worker, addr);
        if (!addr_allowed && !source_address && tapx_ipv6_is_multicast(addr)) {
            const uint8_t *target = tapx_nd_target(packet, len);
            addr_allowed = target != NULL && tapx_ipv6_prefix_match(worker, target);
        }
        if (!addr_allowed && source_address && tapx_ipv6_is_unspecified(addr)) {
            const uint8_t *target = tapx_nd_target(packet, len);
            addr_allowed = target != NULL && tapx_ipv6_prefix_match(worker, target);
        }
        if (!addr_allowed) {
            tapx_count_guard_drop(worker);
            return 0;
        }
    }
    return tapx_tap_nd_guard_allows(worker, packet, len, source_address);
}

static int tapx_tap_arp_guard_allows(struct tapx_worker *worker, const uint8_t *frame,
                                     size_t len, int source_address) {
    if (worker->mac_count == 0 && worker->ipv4_prefix_count == 0) {
        return 1;
    }
    if (len < TAPX_ETH_HEADER_LEN + TAPX_ARP_ETH_IPV4_LEN) {
        tapx_count_guard_drop(worker);
        return 0;
    }

    const uint8_t *arp = frame + TAPX_ETH_HEADER_LEN;
    uint16_t htype = tapx_read_be16(arp);
    uint16_t ptype = tapx_read_be16(arp + 2);
    uint8_t hlen = arp[4];
    uint8_t plen = arp[5];
    if (htype != 1 || ptype != TAPX_ETHERTYPE_IPV4 || hlen != 6 || plen != 4) {
        tapx_count_guard_drop(worker);
        return 0;
    }

    const uint8_t *sha = arp + 8;
    const uint8_t *spa = arp + 14;
    const uint8_t *tha = arp + 18;
    const uint8_t *tpa = arp + 24;
    if (worker->mac_count > 0) {
        const uint8_t *mac = source_address ? sha : tha;
        if (!source_address && tapx_mac_is_zero(mac)) {
            mac = NULL;
        }
        if (mac != NULL && !tapx_mac_match(worker, mac)) {
            tapx_count_guard_drop(worker);
            return 0;
        }
    }
    if (worker->ipv4_prefix_count > 0) {
        uint32_t ip = tapx_read_be32(source_address ? spa : tpa);
        if (!tapx_ipv4_prefix_match(worker, ip)) {
            tapx_count_guard_drop(worker);
            return 0;
        }
    }
    return 1;
}

static int tapx_tap_guard_allows(struct tapx_worker *worker, const uint8_t *frame,
                                 size_t len, int source_address) {
    if (worker->mac_count == 0 && worker->ipv4_prefix_count == 0 && worker->ipv6_prefix_count == 0) {
        return 1;
    }
    if (len < TAPX_ETH_HEADER_LEN) {
        tapx_count_guard_drop(worker);
        return 0;
    }

    if (worker->mac_count > 0) {
        const uint8_t *mac = source_address ? frame + 6 : frame;
        if (!source_address && tapx_mac_is_multicast_or_broadcast(mac)) {
            mac = NULL;
        }
        if (mac != NULL && !tapx_mac_match(worker, mac)) {
            tapx_count_guard_drop(worker);
            return 0;
        }
    }

    uint16_t ether_type = tapx_read_be16(frame + 12);
    if (ether_type == TAPX_ETHERTYPE_IPV4) {
        return tapx_tap_ipv4_guard_allows(worker, frame + TAPX_ETH_HEADER_LEN,
                                          len - TAPX_ETH_HEADER_LEN, source_address);
    }
    if (ether_type == TAPX_ETHERTYPE_IPV6) {
        return tapx_tap_ipv6_guard_allows(worker, frame + TAPX_ETH_HEADER_LEN,
                                          len - TAPX_ETH_HEADER_LEN, source_address);
    }
    if (ether_type == TAPX_ETHERTYPE_ARP) {
        return tapx_tap_arp_guard_allows(worker, frame, len, source_address);
    }
    return 1;
}

static int tapx_frame_guard_allows(struct tapx_worker *worker, const uint8_t *frame,
                                   size_t len, int source_address) {
    if (worker->frame_kind == TAPX_FRAME_TUN) {
        return tapx_tun_ip_guard_allows(worker, frame, len, source_address);
    }
    if (worker->frame_kind == TAPX_FRAME_TAP) {
        return tapx_tap_guard_allows(worker, frame, len, source_address);
    }
    return 1;
}

static int tapx_copy_guard(struct tapx_worker *worker, const struct tapx_address_guard *guard) {
    if (guard == NULL || (guard->ipv4_prefix_count == 0 && guard->ipv6_prefix_count == 0 && guard->mac_count == 0)) {
        return 0;
    }
    if ((guard->ipv4_prefix_count > 0 && guard->ipv4_prefixes == NULL) ||
        (guard->ipv6_prefix_count > 0 && guard->ipv6_prefixes == NULL) ||
        (guard->mac_count > 0 && guard->macs == NULL)) {
        return -EINVAL;
    }
    if (guard->ipv4_prefix_count > SIZE_MAX / sizeof(struct tapx_ipv4_prefix)) {
        return -ENOMEM;
    }
    if (guard->mac_count > SIZE_MAX / sizeof(struct tapx_mac_addr)) {
        return -ENOMEM;
    }
    if (guard->ipv6_prefix_count > SIZE_MAX / sizeof(struct tapx_ipv6_prefix)) {
        return -ENOMEM;
    }
    if (guard->ipv4_prefix_count > 0) {
        worker->ipv4_prefixes = calloc(guard->ipv4_prefix_count, sizeof(struct tapx_ipv4_prefix));
        if (worker->ipv4_prefixes == NULL) {
            return -ENOMEM;
        }
        memcpy(worker->ipv4_prefixes, guard->ipv4_prefixes,
               guard->ipv4_prefix_count * sizeof(struct tapx_ipv4_prefix));
        worker->ipv4_prefix_count = guard->ipv4_prefix_count;
    }
    if (guard->ipv6_prefix_count > 0) {
        worker->ipv6_prefixes = calloc(guard->ipv6_prefix_count, sizeof(struct tapx_ipv6_prefix));
        if (worker->ipv6_prefixes == NULL) {
            return -ENOMEM;
        }
        memcpy(worker->ipv6_prefixes, guard->ipv6_prefixes,
               guard->ipv6_prefix_count * sizeof(struct tapx_ipv6_prefix));
        worker->ipv6_prefix_count = guard->ipv6_prefix_count;
    }
    if (guard->mac_count > 0) {
        worker->macs = calloc(guard->mac_count, sizeof(struct tapx_mac_addr));
        if (worker->macs == NULL) {
            return -ENOMEM;
        }
        memcpy(worker->macs, guard->macs, guard->mac_count * sizeof(struct tapx_mac_addr));
        worker->mac_count = guard->mac_count;
    }
    return 0;
}

static int tapx_copy_vkey(struct tapx_worker *worker, const struct tapx_vkey_guard *vkey) {
    if (vkey == NULL || vkey->value_len == 0) {
        return 0;
    }
    if (vkey->value == NULL || vkey->value_len > TAPX_VKEY_MAX_LEN ||
        vkey->value_len > UINT16_MAX ||
        vkey->value_len > SIZE_MAX - TAPX_VKEY_HEADER_BASE_SIZE) {
        return -EINVAL;
    }
    worker->vkey_value = malloc(vkey->value_len);
    if (worker->vkey_value == NULL) {
        return -ENOMEM;
    }
    memcpy(worker->vkey_value, vkey->value, vkey->value_len);
    worker->vkey_len = vkey->value_len;
    worker->vkey_header_size = (uint32_t)(TAPX_VKEY_HEADER_BASE_SIZE + vkey->value_len);
    return 0;
}

static void tapx_worker_free_buffers(struct tapx_worker *worker) {
    if (worker == NULL) {
        return;
    }
    free(worker->macs);
    free(worker->ipv6_prefixes);
    free(worker->ipv4_prefixes);
    free(worker->vkey_value);
    free(worker->stream_buffer);
    free(worker->buffer);
    free(worker);
}

static int tapx_write_full(int fd, const uint8_t *data, size_t len) {
    size_t offset = 0;
    while (offset < len) {
        ssize_t n = write(fd, data + offset, len - offset);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -errno;
        }
        if (n == 0) {
            return -EPIPE;
        }
        offset += (size_t)n;
    }
    return 0;
}

static void tapx_handle_tun_read(struct tapx_worker *worker) {
    for (;;) {
        uint8_t *payload = worker->buffer + worker->vkey_header_size;
        ssize_t n = read(worker->tun_fd, payload, worker->max_frame_size);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
                return;
            }
            tapx_count_io_drop(worker);
            return;
        }
        if (n == 0) {
            return;
        }
        if (!tapx_frame_guard_allows(worker, payload, (size_t)n, 1)) {
            continue;
        }
        if (!worker->has_peer) {
            tapx_count_io_drop(worker);
            continue;
        }
        tapx_write_vkey_header(worker, worker->buffer);
        size_t wire_len = (size_t)n + worker->vkey_header_size;
        ssize_t sent = sendto(worker->udp_fd, worker->buffer, wire_len, 0,
                              (const struct sockaddr *)&worker->peer_addr,
                              worker->peer_addr_len);
        if (sent != (ssize_t)wire_len) {
            tapx_count_io_drop(worker);
            continue;
        }
        tapx_count_tx(worker, n);
    }
}

static void tapx_handle_udp_read(struct tapx_worker *worker) {
    for (;;) {
        struct sockaddr_storage from;
        socklen_t from_len = sizeof(from);
        ssize_t n = recvfrom(worker->udp_fd, worker->buffer,
                             worker->max_frame_size + worker->vkey_header_size, 0,
                             (struct sockaddr *)&from, &from_len);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
                return;
            }
            tapx_count_io_drop(worker);
            return;
        }
        if (n == 0) {
            return;
        }

        if (worker->peer_mode == TAPX_UDP_PEER_FIXED && worker->has_peer &&
            !tapx_peer_equal(&worker->peer_addr, worker->peer_addr_len, &from, from_len)) {
            tapx_count_io_drop(worker);
            continue;
        }
        if (worker->peer_mode == TAPX_UDP_PEER_LEARN && !worker->has_peer) {
            memcpy(&worker->peer_addr, &from, from_len);
            worker->peer_addr_len = from_len;
            worker->has_peer = 1;
        }
        if (worker->peer_mode == TAPX_UDP_PEER_ANY) {
            memcpy(&worker->peer_addr, &from, from_len);
            worker->peer_addr_len = from_len;
            worker->has_peer = 1;
        }

        const uint8_t *payload = worker->buffer;
        size_t payload_len = (size_t)n;
        if (!tapx_strip_vkey_header(worker, &payload, &payload_len)) {
            continue;
        }

        if (!tapx_frame_guard_allows(worker, payload, payload_len, 0)) {
            continue;
        }

        ssize_t written = write(worker->tun_fd, payload, payload_len);
        if (written != (ssize_t)payload_len) {
            tapx_count_io_drop(worker);
            continue;
        }
        tapx_count_rx(worker, written);
    }
}

static void *tapx_udp_pipe_main(void *arg) {
    struct tapx_worker *worker = (struct tapx_worker *)arg;
    struct epoll_event events[TAPX_EPOLL_MAX_EVENTS];

    for (;;) {
        int n = epoll_wait(worker->epoll_fd, events, TAPX_EPOLL_MAX_EVENTS, -1);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            tapx_count_io_drop(worker);
            continue;
        }
        for (int i = 0; i < n; i++) {
            int fd = events[i].data.fd;
            if (fd == worker->stop_fd) {
                uint64_t value = 0;
                ssize_t ignored = read(worker->stop_fd, &value, sizeof(value));
                (void)ignored;
                return NULL;
            }
            if ((events[i].events & (EPOLLERR | EPOLLHUP)) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
            if (fd == worker->tun_fd) {
                tapx_handle_tun_read(worker);
            } else if (fd == worker->udp_fd) {
                tapx_handle_udp_read(worker);
            }
        }
    }
}

static void tapx_handle_tcp_tun_read(struct tapx_worker *worker) {
    for (;;) {
        uint8_t *payload = worker->buffer + worker->header_size + worker->vkey_header_size;
        ssize_t n = read(worker->tun_fd, payload, worker->max_frame_size);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
                return;
            }
            tapx_count_io_drop(worker);
            return;
        }
        if (n == 0) {
            return;
        }
        if (!tapx_frame_guard_allows(worker, payload, (size_t)n, 1)) {
            continue;
        }
        tapx_write_vkey_header(worker, worker->buffer + worker->header_size);
        size_t wire_payload_len = (size_t)n + worker->vkey_header_size;
        if (worker->length_mode == TAPX_TCP_LENGTH_UINT16) {
            if (wire_payload_len > 65535U) {
                tapx_count_io_drop(worker);
                continue;
            }
            tapx_write_be16(worker->buffer, (uint16_t)wire_payload_len);
        } else {
            tapx_write_be32(worker->buffer, (uint32_t)wire_payload_len);
        }
        int rc = tapx_write_full(worker->tcp_fd, worker->buffer, worker->header_size + wire_payload_len);
        if (rc != 0) {
            tapx_count_io_drop(worker);
            continue;
        }
        tapx_count_tx(worker, n);
    }
}

static int tapx_tcp_frame_length(struct tapx_worker *worker, uint32_t *length) {
    if (worker->stream_len < worker->header_size) {
        return 0;
    }
    if (worker->length_mode == TAPX_TCP_LENGTH_UINT16) {
        *length = tapx_read_be16(worker->stream_buffer);
    } else {
        *length = tapx_read_be32(worker->stream_buffer);
    }
    if (*length > worker->max_frame_size + worker->vkey_header_size) {
        tapx_count_io_drop(worker);
        worker->stream_len = 0;
        return 0;
    }
    return 1;
}

static void tapx_tcp_parse_stream(struct tapx_worker *worker) {
    for (;;) {
        uint32_t frame_len = 0;
        if (!tapx_tcp_frame_length(worker, &frame_len)) {
            return;
        }
        size_t total = worker->header_size + (size_t)frame_len;
        if (worker->stream_len < total) {
            return;
        }
        const uint8_t *payload = worker->stream_buffer + worker->header_size;
        size_t payload_len = frame_len;
        if (!tapx_strip_vkey_header(worker, &payload, &payload_len)) {
            size_t remaining = worker->stream_len - total;
            if (remaining > 0) {
                memmove(worker->stream_buffer, worker->stream_buffer + total, remaining);
            }
            worker->stream_len = remaining;
            continue;
        }
        if (!tapx_frame_guard_allows(worker, payload, payload_len, 0)) {
            size_t remaining = worker->stream_len - total;
            if (remaining > 0) {
                memmove(worker->stream_buffer, worker->stream_buffer + total, remaining);
            }
            worker->stream_len = remaining;
            continue;
        }
        ssize_t written = write(worker->tun_fd, payload, payload_len);
        if (written != (ssize_t)payload_len) {
            tapx_count_io_drop(worker);
        } else {
            tapx_count_rx(worker, written);
        }
        size_t remaining = worker->stream_len - total;
        if (remaining > 0) {
            memmove(worker->stream_buffer, worker->stream_buffer + total, remaining);
        }
        worker->stream_len = remaining;
    }
}

static void tapx_handle_tcp_read(struct tapx_worker *worker) {
    for (;;) {
        if (worker->stream_len == worker->stream_cap) {
            tapx_count_io_drop(worker);
            worker->stream_len = 0;
        }
        ssize_t n = read(worker->tcp_fd, worker->stream_buffer + worker->stream_len,
                         worker->stream_cap - worker->stream_len);
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR) {
                return;
            }
            tapx_count_io_drop(worker);
            return;
        }
        if (n == 0) {
            return;
        }
        worker->stream_len += (size_t)n;
        tapx_tcp_parse_stream(worker);
    }
}

static void *tapx_tcp_pipe_main(void *arg) {
    struct tapx_worker *worker = (struct tapx_worker *)arg;
    struct epoll_event events[TAPX_EPOLL_MAX_EVENTS];

    for (;;) {
        int n = epoll_wait(worker->epoll_fd, events, TAPX_EPOLL_MAX_EVENTS, -1);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            tapx_count_io_drop(worker);
            continue;
        }
        for (int i = 0; i < n; i++) {
            int fd = events[i].data.fd;
            if (fd == worker->stop_fd) {
                uint64_t value = 0;
                ssize_t ignored = read(worker->stop_fd, &value, sizeof(value));
                (void)ignored;
                return NULL;
            }
            if ((events[i].events & (EPOLLERR | EPOLLHUP)) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
            if (fd == worker->tun_fd) {
                tapx_handle_tcp_tun_read(worker);
            } else if (fd == worker->tcp_fd) {
                tapx_handle_tcp_read(worker);
            }
        }
    }
}

uint32_t tapx_fastpath_abi_version(void) {
    return TAPX_FASTPATH_ABI_VERSION;
}

void tapx_fastpath_counters_reset(struct tapx_fastpath_counters *counters) {
    if (counters == NULL) {
        return;
    }

    counters->rx_packets = 0;
    counters->tx_packets = 0;
    counters->rx_bytes = 0;
    counters->tx_bytes = 0;
    counters->drops_guard = 0;
    counters->drops_io = 0;
}

int tapx_udp_pipe_start(const struct tapx_udp_pipe_config *config, struct tapx_worker **out_worker) {
    if (config == NULL || out_worker == NULL) {
        return -EINVAL;
    }
    *out_worker = NULL;
    if (config->tun_fd < 0 || config->udp_fd < 0) {
        return -EINVAL;
    }
    if (config->frame_kind != TAPX_FRAME_TUN && config->frame_kind != TAPX_FRAME_TAP) {
        return -EINVAL;
    }
    if (config->frame_kind != TAPX_FRAME_TAP && config->guard.mac_count > 0) {
        return -EINVAL;
    }
    if (config->peer_mode != TAPX_UDP_PEER_ANY &&
        config->peer_mode != TAPX_UDP_PEER_FIXED &&
        config->peer_mode != TAPX_UDP_PEER_LEARN) {
        return -EINVAL;
    }
    if (config->peer_mode == TAPX_UDP_PEER_FIXED && config->peer_addr_len == 0) {
        return -EINVAL;
    }
    uint32_t max_frame_size = config->max_frame_size;
    if (max_frame_size == 0) {
        max_frame_size = TAPX_DEFAULT_MAX_FRAME_SIZE;
    }

    struct tapx_worker *worker = calloc(1, sizeof(*worker));
    if (worker == NULL) {
        return -ENOMEM;
    }
    worker->tun_fd = config->tun_fd;
    worker->udp_fd = config->udp_fd;
    worker->frame_kind = config->frame_kind;
    worker->max_frame_size = max_frame_size;
    worker->peer_mode = config->peer_mode;
    worker->counters = config->counters;
    int rc = tapx_copy_vkey(worker, &config->vkey);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }
    size_t udp_frame_cap = (size_t)max_frame_size;
    size_t udp_vkey_cap = (size_t)worker->vkey_header_size;
    if (udp_vkey_cap > SIZE_MAX - udp_frame_cap) {
        tapx_worker_free_buffers(worker);
        return -ENOMEM;
    }
    worker->buffer = malloc(udp_frame_cap + udp_vkey_cap);
    if (worker->buffer == NULL) {
        tapx_worker_free_buffers(worker);
        return -ENOMEM;
    }
    rc = tapx_copy_guard(worker, &config->guard);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }
    if (config->peer_addr_len > 0) {
        memcpy(&worker->peer_addr, &config->peer_addr, config->peer_addr_len);
        worker->peer_addr_len = config->peer_addr_len;
        worker->has_peer = 1;
    }

    rc = tapx_set_nonblock(worker->tun_fd);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }
    rc = tapx_set_nonblock(worker->udp_fd);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }

    worker->epoll_fd = epoll_create1(EPOLL_CLOEXEC);
    if (worker->epoll_fd < 0) {
        rc = -errno;
        tapx_worker_free_buffers(worker);
        return rc;
    }
    worker->stop_fd = eventfd(0, EFD_NONBLOCK | EFD_CLOEXEC);
    if (worker->stop_fd < 0) {
        rc = -errno;
        close(worker->epoll_fd);
        tapx_worker_free_buffers(worker);
        return rc;
    }

    rc = tapx_epoll_add(worker->epoll_fd, worker->stop_fd, EPOLLIN);
    if (rc == 0) {
        rc = tapx_epoll_add(worker->epoll_fd, worker->tun_fd, EPOLLIN | EPOLLET);
    }
    if (rc == 0) {
        rc = tapx_epoll_add(worker->epoll_fd, worker->udp_fd, EPOLLIN | EPOLLET);
    }
    if (rc != 0) {
        close(worker->stop_fd);
        close(worker->epoll_fd);
        tapx_worker_free_buffers(worker);
        return rc;
    }

    if (pthread_create(&worker->thread, NULL, tapx_udp_pipe_main, worker) != 0) {
        rc = -errno;
        close(worker->stop_fd);
        close(worker->epoll_fd);
        tapx_worker_free_buffers(worker);
        return rc;
    }

    *out_worker = worker;
    return 0;
}

int tapx_tcp_pipe_start(const struct tapx_tcp_pipe_config *config, struct tapx_worker **out_worker) {
    if (config == NULL || out_worker == NULL) {
        return -EINVAL;
    }
    *out_worker = NULL;
    if (config->tun_fd < 0 || config->tcp_fd < 0) {
        return -EINVAL;
    }
    if (config->frame_kind != TAPX_FRAME_TUN && config->frame_kind != TAPX_FRAME_TAP) {
        return -EINVAL;
    }
    if (config->frame_kind != TAPX_FRAME_TAP && config->guard.mac_count > 0) {
        return -EINVAL;
    }
    if (config->length_mode != TAPX_TCP_LENGTH_UINT16 &&
        config->length_mode != TAPX_TCP_LENGTH_UINT32) {
        return -EINVAL;
    }
    uint32_t max_frame_size = config->max_frame_size;
    if (max_frame_size == 0) {
        max_frame_size = TAPX_DEFAULT_MAX_FRAME_SIZE;
    }
    if (config->length_mode == TAPX_TCP_LENGTH_UINT16 && max_frame_size > 65535U) {
        max_frame_size = 65535U;
    }

    struct tapx_worker *worker = calloc(1, sizeof(*worker));
    if (worker == NULL) {
        return -ENOMEM;
    }
    worker->length_mode = config->length_mode;
    worker->header_size = config->length_mode == TAPX_TCP_LENGTH_UINT16 ? 2U : 4U;
    worker->max_frame_size = max_frame_size;
    int rc = tapx_copy_vkey(worker, &config->vkey);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }
    if (worker->length_mode == TAPX_TCP_LENGTH_UINT16) {
        if (worker->vkey_header_size > UINT16_MAX) {
            tapx_worker_free_buffers(worker);
            return -EINVAL;
        }
        uint32_t max_payload = UINT16_MAX - worker->vkey_header_size;
        if (worker->max_frame_size > max_payload) {
            worker->max_frame_size = max_payload;
        }
    }
    size_t tcp_frame_cap = (size_t)worker->max_frame_size;
    size_t tcp_header_cap = (size_t)worker->header_size;
    size_t tcp_vkey_cap = (size_t)worker->vkey_header_size;
    if (tcp_header_cap > SIZE_MAX - tcp_frame_cap ||
        tcp_vkey_cap > SIZE_MAX - tcp_frame_cap - tcp_header_cap) {
        tapx_worker_free_buffers(worker);
        return -ENOMEM;
    }
    worker->stream_cap = tcp_frame_cap + tcp_header_cap + tcp_vkey_cap;
    worker->buffer = malloc(worker->stream_cap);
    worker->stream_buffer = malloc(worker->stream_cap);
    if (worker->buffer == NULL || worker->stream_buffer == NULL) {
        tapx_worker_free_buffers(worker);
        return -ENOMEM;
    }
    worker->tun_fd = config->tun_fd;
    worker->tcp_fd = config->tcp_fd;
    worker->frame_kind = config->frame_kind;
    worker->counters = config->counters;
    rc = tapx_copy_guard(worker, &config->guard);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }

    rc = tapx_set_nonblock(worker->tun_fd);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }
    rc = tapx_set_nonblock(worker->tcp_fd);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }

    worker->epoll_fd = epoll_create1(EPOLL_CLOEXEC);
    if (worker->epoll_fd < 0) {
        rc = -errno;
        tapx_worker_free_buffers(worker);
        return rc;
    }
    worker->stop_fd = eventfd(0, EFD_NONBLOCK | EFD_CLOEXEC);
    if (worker->stop_fd < 0) {
        rc = -errno;
        close(worker->epoll_fd);
        tapx_worker_free_buffers(worker);
        return rc;
    }
    rc = tapx_epoll_add(worker->epoll_fd, worker->stop_fd, EPOLLIN);
    if (rc == 0) {
        rc = tapx_epoll_add(worker->epoll_fd, worker->tun_fd, EPOLLIN | EPOLLET);
    }
    if (rc == 0) {
        rc = tapx_epoll_add(worker->epoll_fd, worker->tcp_fd, EPOLLIN | EPOLLET);
    }
    if (rc != 0) {
        close(worker->stop_fd);
        close(worker->epoll_fd);
        tapx_worker_free_buffers(worker);
        return rc;
    }
    if (pthread_create(&worker->thread, NULL, tapx_tcp_pipe_main, worker) != 0) {
        rc = -errno;
        close(worker->stop_fd);
        close(worker->epoll_fd);
        tapx_worker_free_buffers(worker);
        return rc;
    }

    *out_worker = worker;
    return 0;
}

int tapx_worker_stop(struct tapx_worker *worker) {
    if (worker == NULL) {
        return 0;
    }
    uint64_t value = 1;
    ssize_t ignored = write(worker->stop_fd, &value, sizeof(value));
    (void)ignored;
    int rc = 0;
    if (pthread_join(worker->thread, NULL) != 0) {
        rc = -errno;
    }
    close(worker->stop_fd);
    close(worker->epoll_fd);
    tapx_worker_free_buffers(worker);
    return rc;
}
