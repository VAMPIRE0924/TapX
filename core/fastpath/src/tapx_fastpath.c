#define _POSIX_C_SOURCE 200809L

#include "tapx_fastpath.h"

#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <time.h>
#include <linux/errqueue.h>
#include <netinet/in.h>
#include <poll.h>
#include <pthread.h>
#include <stddef.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/eventfd.h>
#include <sys/socket.h>
#include <unistd.h>

#define TAPX_DEFAULT_MAX_FRAME_SIZE 65535U
#define TAPX_MAX_FRAME_SIZE 65535U
#define TAPX_EPOLL_MAX_EVENTS 3
#define TAPX_ETH_HEADER_LEN 14U
#define TAPX_ETHERTYPE_IPV4 0x0800U
#define TAPX_ETHERTYPE_ARP 0x0806U
#define TAPX_ETHERTYPE_VLAN 0x8100U
#define TAPX_ETHERTYPE_QINQ 0x88A8U
#define TAPX_ETHERTYPE_VLAN_9100 0x9100U
#define TAPX_ETHERTYPE_PPPOE_DISCOVERY 0x8863U
#define TAPX_ETHERTYPE_PPPOE_SESSION 0x8864U
#define TAPX_ETHERTYPE_IPV6 0x86DDU
#define TAPX_PPP_PROTOCOL_IPV4 0x0021U
#define TAPX_PPP_PROTOCOL_IPV6 0x0057U
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
#define TAPX_SEGMENT_HEADER_SIZE 20U
#define TAPX_SEGMENT_MAX_FRAGMENTS 256U
#define TAPX_REASSEMBLY_SLOTS 8U
#define TAPX_SEGMENT_MAGIC_0 'T'
#define TAPX_SEGMENT_MAGIC_1 'X'
#define TAPX_SEGMENT_MAGIC_2 'S'
#define TAPX_SEGMENT_MAGIC_3 '1'
#define TAPX_SEGMENT_BITMAP_SIZE (TAPX_SEGMENT_MAX_FRAGMENTS / 8U)
#define TAPX_DIAG_HEADER_SIZE 32U
#define TAPX_DIAG_VERSION 1U
#define TAPX_DIAG_PING_REQUEST 1U
#define TAPX_DIAG_PING_RESPONSE 2U
#define TAPX_DIAG_UPLOAD_DATA 3U
#define TAPX_DIAG_UPLOAD_FINISH 4U
#define TAPX_DIAG_UPLOAD_ACK 5U
#define TAPX_DIAG_DOWNLOAD_REQUEST 6U
#define TAPX_DIAG_DOWNLOAD_DATA 7U
#define TAPX_DIAG_DOWNLOAD_FINISH 8U

struct tapx_reassembly_slot {
    uint32_t sequence;
    uint16_t total_len;
    uint16_t fragment_count;
    uint16_t fragment_payload_size;
    uint16_t received_count;
    uint8_t active;
    uint8_t received[TAPX_SEGMENT_BITMAP_SIZE];
    uint8_t *data;
};

struct tapx_rate_pacer {
    uint64_t bits_per_second;
    uint64_t next_ns;
};

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
    uint32_t address_guard_remote;
    uint32_t header_size;
    uint32_t vkey_header_size;
    uint32_t max_datagram_payload;
    uint32_t segment_payload_size;
    uint32_t segment_sequence;
    size_t buffer_capacity;
    uint8_t *buffer;
    uint8_t *frame_buffer;
    uint8_t *reassembly_data;
    struct tapx_reassembly_slot *reassembly_slots;
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
	uint64_t diag_upload_session;
	uint64_t diag_upload_bytes;
    struct tapx_rate_pacer device_to_network_pacer;
    struct tapx_rate_pacer network_to_device_pacer;
    struct tapx_fastpath_counters pending_counters;
    struct tapx_fastpath_counters *counters;
};

static uint64_t tapx_monotonic_ns(void) {
    struct timespec now;
    if (clock_gettime(CLOCK_MONOTONIC, &now) != 0) {
        return 0;
    }
    return (uint64_t)now.tv_sec * 1000000000ULL + (uint64_t)now.tv_nsec;
}

static int tapx_rate_pacer_wait(struct tapx_worker *worker,
                                struct tapx_rate_pacer *pacer,
                                size_t byte_count) {
    if (pacer->bits_per_second == 0 || byte_count == 0) {
        return 0;
    }
    uint64_t numerator = (uint64_t)byte_count * 8000000000ULL;
    uint64_t cost_ns = numerator / pacer->bits_per_second;
    if (numerator % pacer->bits_per_second != 0) {
        cost_ns++;
    }
    uint64_t now = tapx_monotonic_ns();
    if (now == 0) {
        return 0;
    }
    uint64_t burst_ns = cost_ns > 50000000ULL ? cost_ns : 50000000ULL;
    uint64_t floor = now > burst_ns ? now - burst_ns : 0;
    if (pacer->next_ns < floor) {
        pacer->next_ns = floor;
    }
    if (UINT64_MAX - pacer->next_ns < cost_ns) {
        pacer->next_ns = now;
    } else {
        pacer->next_ns += cost_ns;
    }
    while (pacer->next_ns > now) {
        uint64_t delay_ns = pacer->next_ns - now;
        uint64_t timeout64 = (delay_ns + 999999ULL) / 1000000ULL;
        int timeout_ms = timeout64 > (uint64_t)INT_MAX ? INT_MAX : (int)timeout64;
        struct pollfd stop = {.fd = worker->stop_fd, .events = POLLIN, .revents = 0};
        int rc = poll(&stop, 1, timeout_ms);
        if (rc > 0 && (stop.revents & POLLIN) != 0) {
            return 1;
        }
        if (rc < 0 && errno != EINTR) {
            return -errno;
        }
        now = tapx_monotonic_ns();
        if (now == 0) {
            return 0;
        }
    }
    return 0;
}

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
    worker->pending_counters.rx_packets++;
    worker->pending_counters.rx_bytes += (uint64_t)n;
}

static void tapx_count_tx(struct tapx_worker *worker, ssize_t n) {
    if (worker->counters == NULL || n <= 0) {
        return;
    }
    worker->pending_counters.tx_packets++;
    worker->pending_counters.tx_bytes += (uint64_t)n;
}

static void tapx_count_io_drop(struct tapx_worker *worker) {
    if (worker->counters == NULL) {
        return;
    }
    worker->pending_counters.drops_io++;
}

static void tapx_count_guard_drop(struct tapx_worker *worker) {
    if (worker->counters == NULL) {
        return;
    }
    worker->pending_counters.drops_guard++;
}

static void tapx_flush_counters(struct tapx_worker *worker) {
    if (worker->counters == NULL) {
        return;
    }
#define TAPX_FLUSH_COUNTER(name) do { \
        uint64_t value = worker->pending_counters.name; \
        if (value != 0) { \
            worker->pending_counters.name = 0; \
            __atomic_fetch_add(&worker->counters->name, value, __ATOMIC_RELAXED); \
        } \
    } while (0)
    TAPX_FLUSH_COUNTER(rx_packets);
    TAPX_FLUSH_COUNTER(tx_packets);
    TAPX_FLUSH_COUNTER(rx_bytes);
    TAPX_FLUSH_COUNTER(tx_bytes);
    TAPX_FLUSH_COUNTER(drops_guard);
    TAPX_FLUSH_COUNTER(drops_io);
#undef TAPX_FLUSH_COUNTER
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

static uint64_t tapx_read_be64(const uint8_t *p) {
    return ((uint64_t)tapx_read_be32(p) << 32) | tapx_read_be32(p + 4);
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

static void tapx_write_be64(uint8_t *p, uint64_t value) {
    tapx_write_be32(p, (uint32_t)(value >> 32));
    tapx_write_be32(p + 4, (uint32_t)value);
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

static int tapx_diag_header(const uint8_t *payload, size_t len) {
    return len >= TAPX_DIAG_HEADER_SIZE &&
           payload[0] == 'T' && payload[1] == 'X' && payload[2] == 'D' && payload[3] == '1' &&
           payload[4] == TAPX_DIAG_VERSION && tapx_read_be16(payload + 6) == TAPX_DIAG_HEADER_SIZE;
}

static void tapx_diag_write_header(uint8_t *payload, uint8_t op, uint64_t session,
                                   uint32_t sequence, uint32_t duration_ms, uint64_t value) {
    payload[0] = 'T';
    payload[1] = 'X';
    payload[2] = 'D';
    payload[3] = '1';
    payload[4] = TAPX_DIAG_VERSION;
    payload[5] = op;
    tapx_write_be16(payload + 6, TAPX_DIAG_HEADER_SIZE);
    tapx_write_be64(payload + 8, session);
    tapx_write_be32(payload + 16, sequence);
    tapx_write_be32(payload + 20, duration_ms);
    tapx_write_be64(payload + 24, value);
}

static int tapx_diag_send(struct tapx_worker *worker,
                          const struct sockaddr_storage *peer, socklen_t peer_len,
                          uint8_t op, uint64_t session, uint32_t sequence,
                          uint32_t duration_ms, uint64_t value, size_t body_len) {
    size_t wire_len = worker->vkey_header_size + TAPX_DIAG_HEADER_SIZE + body_len;
    if (wire_len > worker->buffer_capacity) {
        return -EMSGSIZE;
    }
    tapx_write_vkey_header(worker, worker->buffer);
    uint8_t *header = worker->buffer + worker->vkey_header_size;
    tapx_diag_write_header(header, op, session, sequence, duration_ms, value);
    if (body_len > 0) {
        memset(header + TAPX_DIAG_HEADER_SIZE, 0xa5, body_len);
    }
    ssize_t sent = sendto(worker->udp_fd, worker->buffer, wire_len, 0,
                          (const struct sockaddr *)peer, peer_len);
    if (sent == (ssize_t)wire_len) {
        return 0;
    }
    return sent < 0 ? -errno : -EIO;
}

static int tapx_handle_udp_diagnostic(struct tapx_worker *worker,
                                      const uint8_t *payload, size_t payload_len,
                                      const struct sockaddr_storage *from, socklen_t from_len) {
    if (!tapx_diag_header(payload, payload_len)) {
        return 0;
    }
    uint8_t op = payload[5];
    uint64_t session = tapx_read_be64(payload + 8);
    uint32_t sequence = tapx_read_be32(payload + 16);
    uint32_t duration_ms = tapx_read_be32(payload + 20);
    uint64_t value = tapx_read_be64(payload + 24);
    switch (op) {
        case TAPX_DIAG_PING_REQUEST:
            (void)tapx_diag_send(worker, from, from_len, TAPX_DIAG_PING_RESPONSE,
                                 session, sequence, 0, 0, 0);
            return 1;
        case TAPX_DIAG_UPLOAD_DATA:
            if (worker->diag_upload_session != session) {
                worker->diag_upload_session = session;
                worker->diag_upload_bytes = 0;
            }
            worker->diag_upload_bytes += payload_len - TAPX_DIAG_HEADER_SIZE;
            return 1;
        case TAPX_DIAG_UPLOAD_FINISH:
            if (worker->diag_upload_session == session) {
                value = worker->diag_upload_bytes;
            } else {
                value = 0;
            }
            (void)tapx_diag_send(worker, from, from_len, TAPX_DIAG_UPLOAD_ACK,
                                 session, sequence, duration_ms, value, 0);
            return 1;
        case TAPX_DIAG_DOWNLOAD_REQUEST: {
            if (duration_ms < 100U) {
                duration_ms = 100U;
            }
            if (duration_ms > 10000U) {
                duration_ms = 10000U;
            }
            size_t overhead = worker->vkey_header_size + TAPX_DIAG_HEADER_SIZE;
            size_t body_len = value > (uint64_t)SIZE_MAX ? 0 : (size_t)value;
            if (body_len == 0 || body_len + overhead > worker->buffer_capacity) {
                body_len = worker->buffer_capacity > overhead ? worker->buffer_capacity - overhead : 0;
            }
            if (body_len == 0) {
                return 1;
            }
            uint64_t started = tapx_monotonic_ns();
            uint64_t deadline = started + (uint64_t)duration_ms * 1000000ULL;
            uint64_t total = 0;
            uint32_t next_sequence = 0;
            while (tapx_monotonic_ns() < deadline) {
                int rc = tapx_diag_send(worker, from, from_len, TAPX_DIAG_DOWNLOAD_DATA,
                                        session, next_sequence++, duration_ms, body_len, body_len);
                if (rc == 0) {
                    total += body_len;
                } else if (rc != -EAGAIN && rc != -EWOULDBLOCK && rc != -EINTR) {
                    break;
                }
            }
            (void)tapx_diag_send(worker, from, from_len, TAPX_DIAG_DOWNLOAD_FINISH,
                                 session, next_sequence, duration_ms, total, 0);
            return 1;
        }
        default:
            return 1;
    }
}

static int tapx_segment_enabled(const struct tapx_worker *worker) {
    return worker->max_datagram_payload > 0;
}

static int tapx_apply_udp_path_mtu(struct tapx_worker *worker, uint32_t path_mtu) {
    if (!tapx_segment_enabled(worker) || path_mtu == 0) {
        return 0;
    }
    uint32_t ip_header_size = worker->peer_addr.ss_family == AF_INET6 ? 40U : 20U;
    uint32_t outer_overhead = ip_header_size + 8U;
    uint32_t control_overhead = worker->vkey_header_size + TAPX_SEGMENT_HEADER_SIZE;
    if (path_mtu <= outer_overhead + control_overhead) {
        return 0;
    }
    uint32_t max_datagram_payload = path_mtu - outer_overhead;
    uint32_t segment_payload_size = max_datagram_payload - control_overhead;
    uint32_t minimum_segment_payload =
        (worker->max_frame_size + TAPX_SEGMENT_MAX_FRAGMENTS - 1U) /
        TAPX_SEGMENT_MAX_FRAGMENTS;
    if (segment_payload_size < minimum_segment_payload ||
        max_datagram_payload >= worker->max_datagram_payload) {
        return 0;
    }
    worker->max_datagram_payload = max_datagram_payload;
    worker->segment_payload_size = segment_payload_size;
    return 1;
}

static uint32_t tapx_udp_socket_path_mtu(const struct tapx_worker *worker) {
    int value = 0;
    socklen_t value_len = sizeof(value);
    int level = worker->peer_addr.ss_family == AF_INET6 ? IPPROTO_IPV6 : IPPROTO_IP;
    int option = worker->peer_addr.ss_family == AF_INET6 ? IPV6_MTU : IP_MTU;
    if (getsockopt(worker->udp_fd, level, option, &value, &value_len) == 0 && value > 0) {
        return (uint32_t)value;
    }
    return 0;
}

static uint32_t tapx_udp_error_path_mtu(struct tapx_worker *worker) {
    uint32_t path_mtu = 0;
    for (;;) {
        uint8_t payload = 0;
        uint8_t control[256];
        struct iovec iov = {.iov_base = &payload, .iov_len = sizeof(payload)};
        struct msghdr message;
        memset(&message, 0, sizeof(message));
        message.msg_iov = &iov;
        message.msg_iovlen = 1;
        message.msg_control = control;
        message.msg_controllen = sizeof(control);
        if (recvmsg(worker->udp_fd, &message, MSG_ERRQUEUE | MSG_DONTWAIT) < 0) {
            break;
        }
        for (struct cmsghdr *header = CMSG_FIRSTHDR(&message); header != NULL;
             header = CMSG_NXTHDR(&message, header)) {
            int is_ipv4 = header->cmsg_level == IPPROTO_IP && header->cmsg_type == IP_RECVERR;
            int is_ipv6 = header->cmsg_level == IPPROTO_IPV6 && header->cmsg_type == IPV6_RECVERR;
            if ((!is_ipv4 && !is_ipv6) || header->cmsg_len < CMSG_LEN(sizeof(struct sock_extended_err))) {
                continue;
            }
            const struct sock_extended_err *extended =
                (const struct sock_extended_err *)CMSG_DATA(header);
            if (extended->ee_errno == EMSGSIZE && extended->ee_info > 0 &&
                (path_mtu == 0 || extended->ee_info < path_mtu)) {
                path_mtu = extended->ee_info;
            }
        }
    }
    if (path_mtu == 0) {
        path_mtu = tapx_udp_socket_path_mtu(worker);
    }
    return path_mtu;
}

static int tapx_refresh_udp_datagram_limit(struct tapx_worker *worker) {
    return tapx_apply_udp_path_mtu(worker, tapx_udp_error_path_mtu(worker));
}

static void tapx_write_segment_header(uint8_t *header, uint32_t sequence,
                                      uint16_t total_len, uint16_t fragment_index,
                                      uint16_t fragment_count, uint16_t fragment_payload_size,
                                      uint16_t fragment_len) {
    header[0] = TAPX_SEGMENT_MAGIC_0;
    header[1] = TAPX_SEGMENT_MAGIC_1;
    header[2] = TAPX_SEGMENT_MAGIC_2;
    header[3] = TAPX_SEGMENT_MAGIC_3;
    tapx_write_be32(header + 4, sequence);
    tapx_write_be16(header + 8, total_len);
    tapx_write_be16(header + 10, fragment_index);
    tapx_write_be16(header + 12, fragment_count);
    tapx_write_be16(header + 14, fragment_payload_size);
    tapx_write_be16(header + 16, fragment_len);
    header[18] = 0;
    header[19] = 0;
}

static int tapx_send_segmented(struct tapx_worker *worker, const uint8_t *frame,
                               size_t frame_len) {
    size_t fragment_payload_size = worker->segment_payload_size;
    size_t fragment_count = (frame_len + fragment_payload_size - 1U) / fragment_payload_size;
    uint32_t sequence = ++worker->segment_sequence;
    for (size_t index = 0; index < fragment_count; index++) {
        size_t offset = index * fragment_payload_size;
        size_t fragment_len = frame_len - offset;
        if (fragment_len > fragment_payload_size) {
            fragment_len = fragment_payload_size;
        }
        tapx_write_vkey_header(worker, worker->buffer);
        uint8_t *header = worker->buffer + worker->vkey_header_size;
        tapx_write_segment_header(header, sequence, (uint16_t)frame_len, (uint16_t)index,
                                  (uint16_t)fragment_count, (uint16_t)fragment_payload_size,
                                  (uint16_t)fragment_len);
        uint8_t *payload = header + TAPX_SEGMENT_HEADER_SIZE;
        memcpy(payload, frame + offset, fragment_len);
        size_t wire_len = worker->vkey_header_size + TAPX_SEGMENT_HEADER_SIZE + fragment_len;
        ssize_t sent = sendto(worker->udp_fd, worker->buffer, wire_len, 0,
                              (const struct sockaddr *)&worker->peer_addr,
                              worker->peer_addr_len);
        if (sent != (ssize_t)wire_len) {
            return sent < 0 ? -errno : -EIO;
        }
    }
    return 0;
}

static int tapx_send_segmented_adaptive(struct tapx_worker *worker, const uint8_t *frame,
                                        size_t frame_len) {
    int result = tapx_send_segmented(worker, frame, frame_len);
    if (result != -EMSGSIZE) {
        return result;
    }
    if (!tapx_refresh_udp_datagram_limit(worker)) {
        return result;
    }
    return tapx_send_segmented(worker, frame, frame_len);
}

// Returns 1 for a complete frame, 0 for a valid incomplete/duplicate fragment,
// and -1 for malformed input.
static int tapx_reassemble_segment(struct tapx_worker *worker, const uint8_t *payload,
                                   size_t payload_len, const uint8_t **frame,
                                   size_t *frame_len) {
    if (payload_len < TAPX_SEGMENT_HEADER_SIZE ||
        payload[0] != TAPX_SEGMENT_MAGIC_0 || payload[1] != TAPX_SEGMENT_MAGIC_1 ||
        payload[2] != TAPX_SEGMENT_MAGIC_2 || payload[3] != TAPX_SEGMENT_MAGIC_3 ||
        payload[18] != 0 || payload[19] != 0) {
        return -1;
    }
    uint32_t sequence = tapx_read_be32(payload + 4);
    uint16_t total_len = tapx_read_be16(payload + 8);
    uint16_t fragment_index = tapx_read_be16(payload + 10);
    uint16_t fragment_count = tapx_read_be16(payload + 12);
    uint16_t fragment_payload_size = tapx_read_be16(payload + 14);
    uint16_t fragment_len = tapx_read_be16(payload + 16);
    if (total_len == 0 || total_len > worker->max_frame_size ||
        fragment_count == 0 || fragment_count > TAPX_SEGMENT_MAX_FRAGMENTS ||
        fragment_index >= fragment_count || fragment_payload_size == 0 || fragment_len == 0) {
        return -1;
    }
    size_t expected_count = ((size_t)total_len + fragment_payload_size - 1U) /
                            fragment_payload_size;
    size_t offset = (size_t)fragment_index * fragment_payload_size;
    if (expected_count != fragment_count || offset >= total_len) {
        return -1;
    }
    size_t expected_len = (size_t)total_len - offset;
    if (expected_len > fragment_payload_size) {
        expected_len = fragment_payload_size;
    }
    if (fragment_len != expected_len ||
        payload_len != TAPX_SEGMENT_HEADER_SIZE + (size_t)fragment_len) {
        return -1;
    }

    struct tapx_reassembly_slot *slot =
        &worker->reassembly_slots[sequence % TAPX_REASSEMBLY_SLOTS];
    if (!slot->active || slot->sequence != sequence || slot->total_len != total_len ||
        slot->fragment_count != fragment_count ||
        slot->fragment_payload_size != fragment_payload_size) {
        slot->sequence = sequence;
        slot->total_len = total_len;
        slot->fragment_count = fragment_count;
        slot->fragment_payload_size = fragment_payload_size;
        slot->received_count = 0;
        slot->active = 1;
        memset(slot->received, 0, sizeof(slot->received));
    }
    uint8_t mask = (uint8_t)(1U << (fragment_index & 7U));
    uint8_t *bitmap = &slot->received[fragment_index >> 3U];
    if ((*bitmap & mask) != 0) {
        return 0;
    }
    memcpy(slot->data + offset, payload + TAPX_SEGMENT_HEADER_SIZE, fragment_len);
    *bitmap |= mask;
    slot->received_count++;
    if (slot->received_count != slot->fragment_count) {
        return 0;
    }
    slot->active = 0;
    *frame = slot->data;
    *frame_len = slot->total_len;
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
            tapx_count_guard_drop(worker);
            return 0;
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
            tapx_count_guard_drop(worker);
            return 0;
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
        return 1;
    }
    tapx_count_guard_drop(worker);
    return 0;
}

static int tapx_tap_ipv4_guard_allows(struct tapx_worker *worker, const uint8_t *packet,
                                       size_t len, int source_address) {
    if (worker->ipv4_prefix_count == 0) {
        if (worker->ipv6_prefix_count == 0) {
            return 1;
        }
        tapx_count_guard_drop(worker);
        return 0;
    }
    if (len < 20) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    if ((packet[0] >> 4) != 4) {
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
    if (worker->ipv6_prefix_count == 0 && worker->ipv4_prefix_count > 0) {
        tapx_count_guard_drop(worker);
        return 0;
    }
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

static int tapx_tap_arp_guard_allows(struct tapx_worker *worker, const uint8_t *arp,
                                      size_t len, int source_address) {
    if (worker->ipv4_prefix_count == 0 && worker->ipv6_prefix_count > 0) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    if (worker->mac_count == 0 && worker->ipv4_prefix_count == 0) {
        return 1;
    }
    if (len < TAPX_ARP_ETH_IPV4_LEN) {
        tapx_count_guard_drop(worker);
        return 0;
    }
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
    size_t payload_offset = TAPX_ETH_HEADER_LEN;
    for (size_t tags = 0; tags < 2U &&
         (ether_type == TAPX_ETHERTYPE_VLAN || ether_type == TAPX_ETHERTYPE_QINQ ||
          ether_type == TAPX_ETHERTYPE_VLAN_9100); tags++) {
        if (len < payload_offset + 4U) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        ether_type = tapx_read_be16(frame + payload_offset + 2U);
        payload_offset += 4U;
    }
    if (len < payload_offset) {
        tapx_count_guard_drop(worker);
        return 0;
    }
    const uint8_t *payload = frame + payload_offset;
    size_t payload_len = len - payload_offset;
    if (ether_type == TAPX_ETHERTYPE_IPV4) {
        return tapx_tap_ipv4_guard_allows(worker, payload, payload_len, source_address);
    }
    if (ether_type == TAPX_ETHERTYPE_IPV6) {
        return tapx_tap_ipv6_guard_allows(worker, payload, payload_len, source_address);
    }
    if (ether_type == TAPX_ETHERTYPE_ARP) {
        return tapx_tap_arp_guard_allows(worker, payload, payload_len, source_address);
    }
    if (ether_type == TAPX_ETHERTYPE_PPPOE_SESSION) {
        if (payload_len < 7U) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        size_t declared_len = tapx_read_be16(payload + 4U);
        if (declared_len == 0U || declared_len > payload_len - 6U) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        size_t protocol_len = (payload[6] & 0x01U) != 0 ? 1U : 2U;
        if (declared_len < protocol_len || payload_len < 6U + protocol_len) {
            tapx_count_guard_drop(worker);
            return 0;
        }
        uint16_t protocol = protocol_len == 1U ? payload[6] : tapx_read_be16(payload + 6U);
        const uint8_t *ppp_payload = payload + 6U + protocol_len;
        size_t ppp_payload_len = declared_len - protocol_len;
        if (protocol == TAPX_PPP_PROTOCOL_IPV4) {
            return tapx_tap_ipv4_guard_allows(worker, ppp_payload, ppp_payload_len, source_address);
        }
        if (protocol == TAPX_PPP_PROTOCOL_IPV6) {
            return tapx_tap_ipv6_guard_allows(worker, ppp_payload, ppp_payload_len, source_address);
        }
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

static int tapx_guard_source_address(const struct tapx_worker *worker,
                                     int device_to_network) {
    if (worker->address_guard_remote != 0U) {
        return device_to_network ? 0 : 1;
    }
    return device_to_network ? 1 : 0;
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
    free(worker->reassembly_slots);
    free(worker->reassembly_data);
    free(worker->frame_buffer);
    free(worker->stream_buffer);
    free(worker->buffer);
    free(worker);
}

static int tapx_wait_tcp_writable(struct tapx_worker *worker) {
    struct pollfd fds[2] = {
        {.fd = worker->tcp_fd, .events = POLLOUT, .revents = 0},
        {.fd = worker->stop_fd, .events = POLLIN, .revents = 0},
    };
    for (;;) {
        int rc = poll(fds, 2, -1);
        if (rc < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -errno;
        }
        if ((fds[1].revents & POLLIN) != 0) {
            return -ECANCELED;
        }
        if ((fds[0].revents & (POLLERR | POLLHUP | POLLNVAL)) != 0) {
            return -EPIPE;
        }
        if ((fds[0].revents & POLLOUT) != 0) {
            return 0;
        }
    }
}

static int tapx_write_full(struct tapx_worker *worker, const uint8_t *data, size_t len) {
    size_t offset = 0;
    while (offset < len) {
        ssize_t n = send(worker->tcp_fd, data + offset, len - offset, MSG_NOSIGNAL);
        if (n < 0) {
            if (errno == EINTR) {
                continue;
            }
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                int rc = tapx_wait_tcp_writable(worker);
                if (rc == 0) {
                    continue;
                }
                return rc;
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
        uint8_t *payload = tapx_segment_enabled(worker)
                               ? worker->frame_buffer
                               : worker->buffer + worker->vkey_header_size;
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
        if (!tapx_frame_guard_allows(worker, payload, (size_t)n,
                                     tapx_guard_source_address(worker, 1))) {
            continue;
        }
        if (!worker->has_peer) {
            tapx_count_io_drop(worker);
            continue;
        }
        if (tapx_segment_enabled(worker)) {
            if (tapx_send_segmented_adaptive(worker, payload, (size_t)n) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
        } else {
            tapx_write_vkey_header(worker, worker->buffer);
            size_t wire_len = (size_t)n + worker->vkey_header_size;
            ssize_t sent = sendto(worker->udp_fd, worker->buffer, wire_len, 0,
                                  (const struct sockaddr *)&worker->peer_addr,
                                  worker->peer_addr_len);
            if (sent != (ssize_t)wire_len) {
                tapx_count_io_drop(worker);
                continue;
            }
        }
        tapx_count_tx(worker, n);
    }
}

static void tapx_handle_udp_read(struct tapx_worker *worker) {
    for (;;) {
        struct sockaddr_storage from;
        socklen_t from_len = sizeof(from);
        ssize_t n = recvfrom(worker->udp_fd, worker->buffer, worker->buffer_capacity, 0,
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

        const uint8_t *payload = worker->buffer;
        size_t payload_len = (size_t)n;
        if (!tapx_strip_vkey_header(worker, &payload, &payload_len)) {
            continue;
        }
        if (tapx_handle_udp_diagnostic(worker, payload, payload_len, &from, from_len)) {
            continue;
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

        if (tapx_segment_enabled(worker)) {
            const uint8_t *frame = NULL;
            size_t frame_len = 0;
            int status = tapx_reassemble_segment(worker, payload, payload_len, &frame, &frame_len);
            if (status < 0) {
                tapx_count_guard_drop(worker);
                continue;
            }
            if (status == 0) {
                continue;
            }
            payload = frame;
            payload_len = frame_len;
        }

        if (!tapx_frame_guard_allows(worker, payload, payload_len,
                                     tapx_guard_source_address(worker, 0))) {
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

static void tapx_handle_tun_read_limited(struct tapx_worker *worker) {
    for (;;) {
        uint8_t *payload = tapx_segment_enabled(worker)
                               ? worker->frame_buffer
                               : worker->buffer + worker->vkey_header_size;
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
        if (!tapx_frame_guard_allows(worker, payload, (size_t)n,
                                     tapx_guard_source_address(worker, 1))) {
            continue;
        }
        if (!worker->has_peer) {
            tapx_count_io_drop(worker);
            continue;
        }
        int pace = tapx_rate_pacer_wait(worker, &worker->device_to_network_pacer, (size_t)n);
        if (pace > 0) {
            return;
        }
        if (pace < 0) {
            tapx_count_io_drop(worker);
        }
        if (tapx_segment_enabled(worker)) {
            if (tapx_send_segmented_adaptive(worker, payload, (size_t)n) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
        } else {
            tapx_write_vkey_header(worker, worker->buffer);
            size_t wire_len = (size_t)n + worker->vkey_header_size;
            ssize_t sent = sendto(worker->udp_fd, worker->buffer, wire_len, 0,
                                  (const struct sockaddr *)&worker->peer_addr,
                                  worker->peer_addr_len);
            if (sent != (ssize_t)wire_len) {
                tapx_count_io_drop(worker);
                continue;
            }
        }
        tapx_count_tx(worker, n);
    }
}

static void tapx_handle_udp_read_limited(struct tapx_worker *worker) {
    for (;;) {
        struct sockaddr_storage from;
        socklen_t from_len = sizeof(from);
        ssize_t n = recvfrom(worker->udp_fd, worker->buffer, worker->buffer_capacity, 0,
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
        const uint8_t *payload = worker->buffer;
        size_t payload_len = (size_t)n;
        if (!tapx_strip_vkey_header(worker, &payload, &payload_len)) {
            continue;
        }
        if (tapx_handle_udp_diagnostic(worker, payload, payload_len, &from, from_len)) {
            continue;
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
        if (tapx_segment_enabled(worker)) {
            const uint8_t *frame = NULL;
            size_t frame_len = 0;
            int status = tapx_reassemble_segment(worker, payload, payload_len, &frame, &frame_len);
            if (status < 0) {
                tapx_count_guard_drop(worker);
                continue;
            }
            if (status == 0) {
                continue;
            }
            payload = frame;
            payload_len = frame_len;
        }
        if (!tapx_frame_guard_allows(worker, payload, payload_len,
                                     tapx_guard_source_address(worker, 0))) {
            continue;
        }
        int pace = tapx_rate_pacer_wait(worker, &worker->network_to_device_pacer, payload_len);
        if (pace > 0) {
            return;
        }
        if (pace < 0) {
            tapx_count_io_drop(worker);
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
        tapx_flush_counters(worker);
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
                tapx_flush_counters(worker);
                return NULL;
            }
            if ((events[i].events & EPOLLHUP) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
            if ((events[i].events & EPOLLERR) != 0) {
                if (fd != worker->udp_fd || !tapx_refresh_udp_datagram_limit(worker)) {
                    tapx_count_io_drop(worker);
                }
                if ((events[i].events & EPOLLIN) == 0) {
                    continue;
                }
            }
            if (fd == worker->tun_fd) {
                tapx_handle_tun_read(worker);
            } else if (fd == worker->udp_fd) {
                tapx_handle_udp_read(worker);
            }
        }
    }
}

static void *tapx_udp_pipe_limited_main(void *arg) {
    struct tapx_worker *worker = (struct tapx_worker *)arg;
    struct epoll_event events[TAPX_EPOLL_MAX_EVENTS];
    for (;;) {
        tapx_flush_counters(worker);
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
                tapx_flush_counters(worker);
                return NULL;
            }
            if ((events[i].events & EPOLLHUP) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
            if ((events[i].events & EPOLLERR) != 0) {
                if (fd != worker->udp_fd || !tapx_refresh_udp_datagram_limit(worker)) {
                    tapx_count_io_drop(worker);
                }
                if ((events[i].events & EPOLLIN) == 0) {
                    continue;
                }
            }
            if (fd == worker->tun_fd) {
                tapx_handle_tun_read_limited(worker);
            } else if (fd == worker->udp_fd) {
                tapx_handle_udp_read_limited(worker);
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
        if (!tapx_frame_guard_allows(worker, payload, (size_t)n,
                                     tapx_guard_source_address(worker, 1))) {
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
        int rc = tapx_write_full(worker, worker->buffer, worker->header_size + wire_payload_len);
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
        if (!tapx_frame_guard_allows(worker, payload, payload_len,
                                     tapx_guard_source_address(worker, 0))) {
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

static void tapx_handle_tcp_tun_read_limited(struct tapx_worker *worker) {
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
        if (!tapx_frame_guard_allows(worker, payload, (size_t)n,
                                     tapx_guard_source_address(worker, 1))) {
            continue;
        }
        int pace = tapx_rate_pacer_wait(worker, &worker->device_to_network_pacer, (size_t)n);
        if (pace > 0) {
            return;
        }
        if (pace < 0) {
            tapx_count_io_drop(worker);
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
        int rc = tapx_write_full(worker, worker->buffer, worker->header_size + wire_payload_len);
        if (rc != 0) {
            tapx_count_io_drop(worker);
            continue;
        }
        tapx_count_tx(worker, n);
    }
}

static void tapx_tcp_parse_stream_limited(struct tapx_worker *worker) {
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
        int allowed = tapx_strip_vkey_header(worker, &payload, &payload_len) &&
                      tapx_frame_guard_allows(worker, payload, payload_len,
                                              tapx_guard_source_address(worker, 0));
        if (allowed) {
            int pace = tapx_rate_pacer_wait(worker, &worker->network_to_device_pacer, payload_len);
            if (pace > 0) {
                return;
            }
            if (pace < 0) {
                tapx_count_io_drop(worker);
            }
            ssize_t written = write(worker->tun_fd, payload, payload_len);
            if (written != (ssize_t)payload_len) {
                tapx_count_io_drop(worker);
            } else {
                tapx_count_rx(worker, written);
            }
        }
        size_t remaining = worker->stream_len - total;
        if (remaining > 0) {
            memmove(worker->stream_buffer, worker->stream_buffer + total, remaining);
        }
        worker->stream_len = remaining;
    }
}

static void tapx_handle_tcp_read_limited(struct tapx_worker *worker) {
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
        tapx_tcp_parse_stream_limited(worker);
    }
}

static void *tapx_tcp_pipe_main(void *arg) {
    struct tapx_worker *worker = (struct tapx_worker *)arg;
    struct epoll_event events[TAPX_EPOLL_MAX_EVENTS];

    for (;;) {
        tapx_flush_counters(worker);
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
                tapx_flush_counters(worker);
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

static void *tapx_tcp_pipe_limited_main(void *arg) {
    struct tapx_worker *worker = (struct tapx_worker *)arg;
    struct epoll_event events[TAPX_EPOLL_MAX_EVENTS];
    for (;;) {
        tapx_flush_counters(worker);
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
                tapx_flush_counters(worker);
                return NULL;
            }
            if ((events[i].events & (EPOLLERR | EPOLLHUP)) != 0) {
                tapx_count_io_drop(worker);
                continue;
            }
            if (fd == worker->tun_fd) {
                tapx_handle_tcp_tun_read_limited(worker);
            } else if (fd == worker->tcp_fd) {
                tapx_handle_tcp_read_limited(worker);
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

    __atomic_store_n(&counters->rx_packets, 0U, __ATOMIC_RELAXED);
    __atomic_store_n(&counters->tx_packets, 0U, __ATOMIC_RELAXED);
    __atomic_store_n(&counters->rx_bytes, 0U, __ATOMIC_RELAXED);
    __atomic_store_n(&counters->tx_bytes, 0U, __ATOMIC_RELAXED);
    __atomic_store_n(&counters->drops_guard, 0U, __ATOMIC_RELAXED);
    __atomic_store_n(&counters->drops_io, 0U, __ATOMIC_RELAXED);
}

void tapx_fastpath_counters_snapshot(const struct tapx_fastpath_counters *counters,
                                     struct tapx_fastpath_counters *snapshot) {
    if (snapshot == NULL) {
        return;
    }
    if (counters == NULL) {
        memset(snapshot, 0, sizeof(*snapshot));
        return;
    }
    snapshot->rx_packets = __atomic_load_n(&counters->rx_packets, __ATOMIC_RELAXED);
    snapshot->tx_packets = __atomic_load_n(&counters->tx_packets, __ATOMIC_RELAXED);
    snapshot->rx_bytes = __atomic_load_n(&counters->rx_bytes, __ATOMIC_RELAXED);
    snapshot->tx_bytes = __atomic_load_n(&counters->tx_bytes, __ATOMIC_RELAXED);
    snapshot->drops_guard = __atomic_load_n(&counters->drops_guard, __ATOMIC_RELAXED);
    snapshot->drops_io = __atomic_load_n(&counters->drops_io, __ATOMIC_RELAXED);
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
    if (config->address_guard_remote > 1U) {
        return -EINVAL;
    }
    if (config->peer_mode == TAPX_UDP_PEER_FIXED && config->peer_addr_len == 0) {
        return -EINVAL;
    }
    if ((size_t)config->peer_addr_len > sizeof(config->peer_addr)) {
        return -EINVAL;
    }
    uint32_t max_frame_size = config->max_frame_size;
    if (max_frame_size == 0) {
        max_frame_size = TAPX_DEFAULT_MAX_FRAME_SIZE;
    }
    if (max_frame_size > TAPX_MAX_FRAME_SIZE) {
        return -EINVAL;
    }

    struct tapx_worker *worker = calloc(1, sizeof(*worker));
    if (worker == NULL) {
        return -ENOMEM;
    }
    worker->tun_fd = config->tun_fd;
    worker->udp_fd = config->udp_fd;
    worker->frame_kind = config->frame_kind;
    worker->max_frame_size = max_frame_size;
    worker->max_datagram_payload = config->max_datagram_payload;
    worker->peer_mode = config->peer_mode;
    worker->address_guard_remote = config->address_guard_remote;
    worker->device_to_network_pacer.bits_per_second = config->device_to_network_rate_bps;
    worker->network_to_device_pacer.bits_per_second = config->network_to_device_rate_bps;
    worker->counters = config->counters;
    int rc = tapx_copy_vkey(worker, &config->vkey);
    if (rc != 0) {
        tapx_worker_free_buffers(worker);
        return rc;
    }
    size_t udp_frame_cap = (size_t)max_frame_size;
    size_t udp_vkey_cap = (size_t)worker->vkey_header_size;
    size_t wire_cap = udp_frame_cap + udp_vkey_cap;
    if (config->max_datagram_payload > 0) {
        if (max_frame_size > UINT16_MAX || config->max_datagram_payload > UINT16_MAX ||
            config->max_datagram_payload <= worker->vkey_header_size + TAPX_SEGMENT_HEADER_SIZE) {
            tapx_worker_free_buffers(worker);
            return -EINVAL;
        }
        worker->segment_payload_size = config->max_datagram_payload -
                                       worker->vkey_header_size - TAPX_SEGMENT_HEADER_SIZE;
        size_t max_fragments = (udp_frame_cap + worker->segment_payload_size - 1U) /
                               worker->segment_payload_size;
        if (max_fragments == 0 || max_fragments > TAPX_SEGMENT_MAX_FRAGMENTS ||
            udp_frame_cap > SIZE_MAX / TAPX_REASSEMBLY_SLOTS) {
            tapx_worker_free_buffers(worker);
            return -EINVAL;
        }
        size_t receive_cap = udp_frame_cap + udp_vkey_cap + TAPX_SEGMENT_HEADER_SIZE;
        wire_cap = config->max_datagram_payload;
        if (receive_cap > wire_cap) {
            wire_cap = receive_cap;
        }
        worker->frame_buffer = malloc(udp_frame_cap);
        worker->reassembly_slots = calloc(TAPX_REASSEMBLY_SLOTS,
                                          sizeof(struct tapx_reassembly_slot));
        worker->reassembly_data = malloc(udp_frame_cap * TAPX_REASSEMBLY_SLOTS);
        if (worker->frame_buffer == NULL || worker->reassembly_slots == NULL ||
            worker->reassembly_data == NULL) {
            tapx_worker_free_buffers(worker);
            return -ENOMEM;
        }
        for (size_t i = 0; i < TAPX_REASSEMBLY_SLOTS; i++) {
            worker->reassembly_slots[i].data = worker->reassembly_data + i * udp_frame_cap;
        }
    } else if (udp_vkey_cap > SIZE_MAX - udp_frame_cap) {
        tapx_worker_free_buffers(worker);
        return -ENOMEM;
    }
    worker->buffer = malloc(wire_cap);
    if (worker->buffer == NULL) {
        tapx_worker_free_buffers(worker);
        return -ENOMEM;
    }
    worker->buffer_capacity = wire_cap;
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

    void *(*udp_main)(void *) =
        config->device_to_network_rate_bps == 0 && config->network_to_device_rate_bps == 0
            ? tapx_udp_pipe_main
            : tapx_udp_pipe_limited_main;
    int thread_rc = pthread_create(&worker->thread, NULL, udp_main, worker);
    if (thread_rc != 0) {
        rc = -thread_rc;
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
    if (config->address_guard_remote > 1U) {
        return -EINVAL;
    }
    uint32_t max_frame_size = config->max_frame_size;
    if (max_frame_size == 0) {
        max_frame_size = TAPX_DEFAULT_MAX_FRAME_SIZE;
    }
    if (max_frame_size > TAPX_MAX_FRAME_SIZE) {
        return -EINVAL;
    }
    if (config->length_mode == TAPX_TCP_LENGTH_UINT16 && max_frame_size > 65535U) {
        max_frame_size = 65535U;
    }

    struct tapx_worker *worker = calloc(1, sizeof(*worker));
    if (worker == NULL) {
        return -ENOMEM;
    }
    worker->length_mode = config->length_mode;
    worker->address_guard_remote = config->address_guard_remote;
    worker->device_to_network_pacer.bits_per_second = config->device_to_network_rate_bps;
    worker->network_to_device_pacer.bits_per_second = config->network_to_device_rate_bps;
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
    void *(*tcp_main)(void *) =
        config->device_to_network_rate_bps == 0 && config->network_to_device_rate_bps == 0
            ? tapx_tcp_pipe_main
            : tapx_tcp_pipe_limited_main;
    int thread_rc = pthread_create(&worker->thread, NULL, tcp_main, worker);
    if (thread_rc != 0) {
        rc = -thread_rc;
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
    int thread_rc = pthread_join(worker->thread, NULL);
    if (thread_rc != 0) {
        rc = -thread_rc;
    }
    close(worker->stop_fd);
    close(worker->epoll_fd);
    tapx_worker_free_buffers(worker);
    return rc;
}
