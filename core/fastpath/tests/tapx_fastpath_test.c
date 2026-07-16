#include "tapx_fastpath.h"

#include <arpa/inet.h>
#include <errno.h>
#include <poll.h>
#include <stdio.h>
#include <stdint.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

static int expect(int condition, const char *message) {
    if (!condition) {
        fprintf(stderr, "FAIL: %s\n", message);
        return 1;
    }
    return 0;
}

static int expect_no_data(int fd, const char *message) {
    struct pollfd pfd;
    memset(&pfd, 0, sizeof(pfd));
    pfd.fd = fd;
    pfd.events = POLLIN;
    int rc;
    do {
        rc = poll(&pfd, 1, 100);
    } while (rc < 0 && errno == EINTR);
    if (rc < 0) {
        perror("poll");
        return 1;
    }
    return expect(rc == 0, message);
}

static uint16_t read_be16(const unsigned char *p);

static void make_ipv4_packet(unsigned char packet[20],
                             unsigned char s0, unsigned char s1, unsigned char s2, unsigned char s3,
                             unsigned char d0, unsigned char d1, unsigned char d2, unsigned char d3) {
    memset(packet, 0, 20);
    packet[0] = 0x45;
    packet[2] = 0x00;
    packet[3] = 0x14;
    packet[8] = 64;
    packet[9] = 59;
    packet[12] = s0;
    packet[13] = s1;
    packet[14] = s2;
    packet[15] = s3;
    packet[16] = d0;
    packet[17] = d1;
    packet[18] = d2;
    packet[19] = d3;
}

static size_t make_vkey_payload(unsigned char *out, const unsigned char *key,
                                size_t key_len, const unsigned char *payload,
                                size_t payload_len) {
    out[0] = 'T';
    out[1] = 'X';
    out[2] = 'V';
    out[3] = '1';
    out[4] = (unsigned char)(key_len >> 8);
    out[5] = (unsigned char)(key_len & 0xff);
    out[6] = 0;
    out[7] = 0;
    memcpy(out + 8, key, key_len);
    memcpy(out + 8 + key_len, payload, payload_len);
    return 8 + key_len + payload_len;
}

static void make_tap_ipv4_frame(unsigned char frame[34],
                                const unsigned char dst[6], const unsigned char src[6],
                                unsigned char s0, unsigned char s1, unsigned char s2, unsigned char s3,
                                unsigned char d0, unsigned char d1, unsigned char d2, unsigned char d3) {
    memcpy(frame, dst, 6);
    memcpy(frame + 6, src, 6);
    frame[12] = 0x08;
    frame[13] = 0x00;
    make_ipv4_packet(frame + 14, s0, s1, s2, s3, d0, d1, d2, d3);
}

static void make_tap_vlan_ipv4_frame(unsigned char frame[38],
                                     const unsigned char dst[6], const unsigned char src[6],
                                     unsigned char source_third_octet) {
    memcpy(frame, dst, 6);
    memcpy(frame + 6, src, 6);
    frame[12] = 0x81;
    frame[13] = 0x00;
    frame[14] = 0x00;
    frame[15] = 0x64;
    frame[16] = 0x08;
    frame[17] = 0x00;
    make_ipv4_packet(frame + 18, 10, 0, source_third_octet, 2, 10, 0, 0, 3);
}

static void make_tap_pppoe_ipv4_frame(unsigned char frame[42],
                                      const unsigned char dst[6], const unsigned char src[6],
                                      unsigned char source_third_octet) {
    memcpy(frame, dst, 6);
    memcpy(frame + 6, src, 6);
    frame[12] = 0x88;
    frame[13] = 0x64;
    frame[14] = 0x11;
    frame[15] = 0x00;
    frame[16] = 0x00;
    frame[17] = 0x01;
    frame[18] = 0x00;
    frame[19] = 0x16;
    frame[20] = 0x00;
    frame[21] = 0x21;
    make_ipv4_packet(frame + 22, 10, 0, source_third_octet, 2, 10, 0, 0, 3);
}

static void make_tap_arp_frame(unsigned char frame[42],
                               const unsigned char dst[6], const unsigned char src[6],
                               const unsigned char sha[6], unsigned char spa0, unsigned char spa1,
                               unsigned char spa2, unsigned char spa3,
                               const unsigned char tha[6], unsigned char tpa0, unsigned char tpa1,
                               unsigned char tpa2, unsigned char tpa3) {
    memcpy(frame, dst, 6);
    memcpy(frame + 6, src, 6);
    frame[12] = 0x08;
    frame[13] = 0x06;
    memset(frame + 14, 0, 28);
    frame[14] = 0x00;
    frame[15] = 0x01;
    frame[16] = 0x08;
    frame[17] = 0x00;
    frame[18] = 0x06;
    frame[19] = 0x04;
    frame[20] = 0x00;
    frame[21] = 0x01;
    memcpy(frame + 22, sha, 6);
    frame[28] = spa0;
    frame[29] = spa1;
    frame[30] = spa2;
    frame[31] = spa3;
    memcpy(frame + 32, tha, 6);
    frame[38] = tpa0;
    frame[39] = tpa1;
    frame[40] = tpa2;
    frame[41] = tpa3;
}

static void make_ipv6_packet(unsigned char packet[40], const unsigned char src[16],
                             const unsigned char dst[16], unsigned char next_header) {
    memset(packet, 0, 40);
    packet[0] = 0x60;
    packet[6] = next_header;
    packet[7] = 64;
    memcpy(packet + 8, src, 16);
    memcpy(packet + 24, dst, 16);
}

static void make_tap_ipv6_frame(unsigned char frame[54],
                                const unsigned char dst_mac[6], const unsigned char src_mac[6],
                                const unsigned char src_ip[16], const unsigned char dst_ip[16]) {
    memcpy(frame, dst_mac, 6);
    memcpy(frame + 6, src_mac, 6);
    frame[12] = 0x86;
    frame[13] = 0xdd;
    make_ipv6_packet(frame + 14, src_ip, dst_ip, 59);
}

static void make_tap_nd_ns_frame(unsigned char frame[86],
                                 const unsigned char dst_mac[6], const unsigned char src_mac[6],
                                 const unsigned char src_ip[16], const unsigned char dst_ip[16],
                                 const unsigned char target_ip[16], const unsigned char slla[6]) {
    memcpy(frame, dst_mac, 6);
    memcpy(frame + 6, src_mac, 6);
    frame[12] = 0x86;
    frame[13] = 0xdd;
    make_ipv6_packet(frame + 14, src_ip, dst_ip, 58);
    frame[18] = 0x00;
    frame[19] = 0x20;
    unsigned char *icmp = frame + 14 + 40;
    memset(icmp, 0, 32);
    icmp[0] = 135;
    memcpy(icmp + 8, target_ip, 16);
    icmp[24] = 1;
    icmp[25] = 1;
    memcpy(icmp + 26, slla, 6);
}

static void ipv6_prefix_64(struct tapx_ipv6_prefix *prefix, const unsigned char network[16]) {
    memset(prefix, 0, sizeof(*prefix));
    memcpy(prefix->network, network, 16);
    for (int i = 0; i < 8; i++) {
        prefix->mask[i] = 0xff;
    }
    for (int i = 0; i < 16; i++) {
        prefix->network[i] &= prefix->mask[i];
    }
}

static int test_abi_and_counters(void) {
    struct tapx_fastpath_counters counters;
    memset(&counters, 0xff, sizeof(counters));
    tapx_fastpath_counters_reset(&counters);
    if (expect(tapx_fastpath_abi_version() == TAPX_FASTPATH_ABI_VERSION, "ABI version")) {
        return 1;
    }
    if (expect(counters.rx_packets == 0 && counters.tx_packets == 0 &&
               counters.rx_bytes == 0 && counters.tx_bytes == 0 &&
               counters.drops_guard == 0 && counters.drops_io == 0,
               "counters reset")) {
        return 1;
    }
    return 0;
}

static int test_udp_pipe_rejects_bad_config(void) {
    struct tapx_worker *worker = NULL;
    int rc = tapx_udp_pipe_start(NULL, &worker);
    if (expect(rc < 0, "NULL config should fail")) {
        return 1;
    }

    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = -1;
    config.udp_fd = -1;
    config.frame_kind = TAPX_FRAME_TUN;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(rc < 0, "invalid fd config should fail")) {
        return 1;
    }
    return 0;
}

static int test_udp_pipe_starts_and_stops(void) {
    int tun_pair[2] = {-1, -1};
    int udp_fd = -1;
    int peer_fd = -1;
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tun_pair) != 0) {
        perror("socketpair");
        goto out;
    }
    udp_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    peer_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    if (udp_fd < 0 || peer_fd < 0) {
        perror("socket");
        goto out;
    }

    struct sockaddr_in udp_addr;
    memset(&udp_addr, 0, sizeof(udp_addr));
    udp_addr.sin_family = AF_INET;
    udp_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    udp_addr.sin_port = 0;
    if (bind(udp_fd, (struct sockaddr *)&udp_addr, sizeof(udp_addr)) != 0) {
        perror("bind udp");
        goto out;
    }
    socklen_t udp_len = sizeof(udp_addr);
    if (getsockname(udp_fd, (struct sockaddr *)&udp_addr, &udp_len) != 0) {
        perror("getsockname udp");
        goto out;
    }

    struct sockaddr_in peer_addr;
    memset(&peer_addr, 0, sizeof(peer_addr));
    peer_addr.sin_family = AF_INET;
    peer_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    peer_addr.sin_port = 0;
    if (bind(peer_fd, (struct sockaddr *)&peer_addr, sizeof(peer_addr)) != 0) {
        perror("bind peer");
        goto out;
    }
    socklen_t peer_len = sizeof(peer_addr);
    if (getsockname(peer_fd, (struct sockaddr *)&peer_addr, &peer_len) != 0) {
        perror("getsockname peer");
        goto out;
    }

    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tun_pair[0];
    config.udp_fd = udp_fd;
    config.frame_kind = TAPX_FRAME_TUN;
    config.max_frame_size = 2048;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    memcpy(&config.peer_addr, &peer_addr, sizeof(peer_addr));
    config.peer_addr_len = sizeof(peer_addr);
    config.counters = &counters;

    int start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "worker start")) {
        goto out;
    }

    const char packet[] = "abc";
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write tun peer");
        goto out;
    }
    char rx[16];
    ssize_t n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == (ssize_t)sizeof(packet), "udp receive from worker")) {
        goto out;
    }
    if (expect(memcmp(rx, packet, sizeof(packet)) == 0, "udp payload")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.tx_packets == 1 && counters.tx_bytes == sizeof(packet),
               "tx counters")) {
        goto out;
    }

    worker = NULL;
    tapx_fastpath_counters_reset(&counters);
    start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "worker restart")) {
        goto out;
    }

    const char incoming[] = "xyz";
    ssize_t sent = sendto(peer_fd, incoming, sizeof(incoming), 0,
                          (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(incoming)) {
        perror("sendto worker");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)sizeof(incoming), "tun receive from worker")) {
        goto out;
    }
    if (expect(memcmp(rx, incoming, sizeof(incoming)) == 0, "tun payload")) {
        goto out;
    }
    if (expect(tapx_worker_stop(worker) == 0, "worker second stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.rx_packets == 1 && counters.rx_bytes == sizeof(incoming),
               "rx counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tun_pair[0] >= 0) {
        close(tun_pair[0]);
    }
    if (tun_pair[1] >= 0) {
        close(tun_pair[1]);
    }
    if (udp_fd >= 0) {
        close(udp_fd);
    }
    if (peer_fd >= 0) {
        close(peer_fd);
    }
    return rc;
}

static int test_udp_pipe_vkey_header(void) {
    int tun_pair[2] = {-1, -1};
    int udp_fd = -1;
    int peer_fd = -1;
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tun_pair) != 0) {
        perror("socketpair");
        goto out;
    }
    udp_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    peer_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    if (udp_fd < 0 || peer_fd < 0) {
        perror("socket");
        goto out;
    }

    struct sockaddr_in udp_addr;
    memset(&udp_addr, 0, sizeof(udp_addr));
    udp_addr.sin_family = AF_INET;
    udp_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    udp_addr.sin_port = 0;
    if (bind(udp_fd, (struct sockaddr *)&udp_addr, sizeof(udp_addr)) != 0) {
        perror("bind udp");
        goto out;
    }
    socklen_t udp_len = sizeof(udp_addr);
    if (getsockname(udp_fd, (struct sockaddr *)&udp_addr, &udp_len) != 0) {
        perror("getsockname udp");
        goto out;
    }

    struct sockaddr_in peer_addr;
    memset(&peer_addr, 0, sizeof(peer_addr));
    peer_addr.sin_family = AF_INET;
    peer_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    peer_addr.sin_port = 0;
    if (bind(peer_fd, (struct sockaddr *)&peer_addr, sizeof(peer_addr)) != 0) {
        perror("bind peer");
        goto out;
    }
    socklen_t peer_len = sizeof(peer_addr);
    if (getsockname(peer_fd, (struct sockaddr *)&peer_addr, &peer_len) != 0) {
        perror("getsockname peer");
        goto out;
    }

    const unsigned char key[] = "vk-demo";
    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tun_pair[0];
    config.udp_fd = udp_fd;
    config.frame_kind = TAPX_FRAME_TUN;
    config.max_frame_size = 2048;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    memcpy(&config.peer_addr, &peer_addr, sizeof(peer_addr));
    config.peer_addr_len = sizeof(peer_addr);
    config.vkey.value = key;
    config.vkey.value_len = sizeof(key) - 1;
    config.counters = &counters;

    int start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "vkey udp worker start")) {
        goto out;
    }

    unsigned char packet[20];
    make_ipv4_packet(packet, 10, 0, 0, 2, 10, 0, 0, 3);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write vkey packet");
        goto out;
    }
    unsigned char rx[128];
    ssize_t n = recv(peer_fd, rx, sizeof(rx), 0);
    size_t want_len = 8 + sizeof(key) - 1 + sizeof(packet);
    if (expect(n == (ssize_t)want_len, "udp vkey wire length")) {
        goto out;
    }
    if (expect(memcmp(rx, "TXV1", 4) == 0 && read_be16(rx + 4) == sizeof(key) - 1,
               "udp vkey wire header")) {
        goto out;
    }
    if (expect(memcmp(rx + 8, key, sizeof(key) - 1) == 0 &&
               memcmp(rx + 8 + sizeof(key) - 1, packet, sizeof(packet)) == 0,
               "udp vkey wire payload")) {
        goto out;
    }

    ssize_t sent = sendto(peer_fd, packet, sizeof(packet), 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send missing vkey");
        goto out;
    }
    if (expect_no_data(tun_pair[1], "udp missing vkey should not reach tun")) {
        goto out;
    }

    unsigned char wire[128];
    size_t wire_len = make_vkey_payload(wire, key, sizeof(key) - 1, packet, sizeof(packet));
    sent = sendto(peer_fd, wire, wire_len, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)wire_len) {
        perror("send vkey payload");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)sizeof(packet), "udp vkey payload reaches tun")) {
        goto out;
    }
    if (expect(memcmp(rx, packet, sizeof(packet)) == 0, "udp stripped vkey payload")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "vkey udp worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 1 && counters.tx_packets == 1 && counters.rx_packets == 1,
               "udp vkey counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tun_pair[0] >= 0) {
        close(tun_pair[0]);
    }
    if (tun_pair[1] >= 0) {
        close(tun_pair[1]);
    }
    if (udp_fd >= 0) {
        close(udp_fd);
    }
    if (peer_fd >= 0) {
        close(peer_fd);
    }
    return rc;
}

static int test_udp_pipe_tun_ipv4_guard(void) {
    int tun_pair[2] = {-1, -1};
    int udp_fd = -1;
    int peer_fd = -1;
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tun_pair) != 0) {
        perror("socketpair");
        goto out;
    }
    udp_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    peer_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    if (udp_fd < 0 || peer_fd < 0) {
        perror("socket");
        goto out;
    }

    struct sockaddr_in udp_addr;
    memset(&udp_addr, 0, sizeof(udp_addr));
    udp_addr.sin_family = AF_INET;
    udp_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    udp_addr.sin_port = 0;
    if (bind(udp_fd, (struct sockaddr *)&udp_addr, sizeof(udp_addr)) != 0) {
        perror("bind udp");
        goto out;
    }
    socklen_t udp_len = sizeof(udp_addr);
    if (getsockname(udp_fd, (struct sockaddr *)&udp_addr, &udp_len) != 0) {
        perror("getsockname udp");
        goto out;
    }

    struct sockaddr_in peer_addr;
    memset(&peer_addr, 0, sizeof(peer_addr));
    peer_addr.sin_family = AF_INET;
    peer_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    peer_addr.sin_port = 0;
    if (bind(peer_fd, (struct sockaddr *)&peer_addr, sizeof(peer_addr)) != 0) {
        perror("bind peer");
        goto out;
    }
    socklen_t peer_len = sizeof(peer_addr);
    if (getsockname(peer_fd, (struct sockaddr *)&peer_addr, &peer_len) != 0) {
        perror("getsockname peer");
        goto out;
    }

    struct tapx_ipv4_prefix prefixes[1];
    prefixes[0].network = 0x0a000000U;
    prefixes[0].mask = 0xffffff00U;

    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tun_pair[0];
    config.udp_fd = udp_fd;
    config.frame_kind = TAPX_FRAME_TUN;
    config.max_frame_size = 2048;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    memcpy(&config.peer_addr, &peer_addr, sizeof(peer_addr));
    config.peer_addr_len = sizeof(peer_addr);
    config.guard.ipv4_prefixes = prefixes;
    config.guard.ipv4_prefix_count = 1;
    config.counters = &counters;

    int start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "udp guard worker start")) {
        goto out;
    }

    unsigned char packet[20];
    make_ipv4_packet(packet, 10, 0, 1, 2, 10, 0, 0, 3);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write disallowed tun packet");
        goto out;
    }
    if (expect_no_data(peer_fd, "disallowed IPv4 source should not reach udp peer")) {
        goto out;
    }

    const unsigned char v6_src[16] = {0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2};
    const unsigned char v6_dst[16] = {0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3};
    unsigned char ipv6_packet[40];
    make_ipv6_packet(ipv6_packet, v6_src, v6_dst, 59);
    if (write(tun_pair[1], ipv6_packet, sizeof(ipv6_packet)) != (ssize_t)sizeof(ipv6_packet)) {
        perror("write unlisted IPv6 family");
        goto out;
    }
    if (expect_no_data(peer_fd, "unlisted IPv6 family should not reach udp peer")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 0, 2, 10, 0, 1, 3);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write allowed tun packet");
        goto out;
    }
    unsigned char rx[64];
    ssize_t n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == (ssize_t)sizeof(packet), "allowed IPv4 source reaches udp peer")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 1, 3, 10, 0, 1, 2);
    ssize_t sent = sendto(peer_fd, packet, sizeof(packet), 0,
                          (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send disallowed dst");
        goto out;
    }
    if (expect_no_data(tun_pair[1], "disallowed IPv4 destination should not reach tun")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 1, 3, 10, 0, 0, 2);
    sent = sendto(peer_fd, packet, sizeof(packet), 0,
                  (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send allowed dst");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)sizeof(packet), "allowed IPv4 destination reaches tun")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "udp guard worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 3 && counters.tx_packets == 1 && counters.rx_packets == 1,
               "udp guard counters")) {
        goto out;
    }

    tapx_fastpath_counters_reset(&counters);
    config.address_guard_remote = 1;
    start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "udp remote guard worker start")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 0, 2, 10, 0, 1, 3);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write remote disallowed destination");
        goto out;
    }
    if (expect_no_data(peer_fd, "remote guard disallowed IPv4 destination should not reach peer")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 1, 2, 10, 0, 0, 3);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write remote allowed destination");
        goto out;
    }
    n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == (ssize_t)sizeof(packet), "remote guard allowed IPv4 destination reaches peer")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 1, 3, 10, 0, 0, 2);
    sent = sendto(peer_fd, packet, sizeof(packet), 0,
                  (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send remote disallowed source");
        goto out;
    }
    if (expect_no_data(tun_pair[1], "remote guard disallowed IPv4 source should not reach tun")) {
        goto out;
    }

    make_ipv4_packet(packet, 10, 0, 0, 3, 10, 0, 1, 2);
    sent = sendto(peer_fd, packet, sizeof(packet), 0,
                  (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send remote allowed source");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)sizeof(packet), "remote guard allowed IPv4 source reaches tun")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "udp remote guard worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 2 && counters.tx_packets == 1 && counters.rx_packets == 1,
               "udp remote guard counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tun_pair[0] >= 0) {
        close(tun_pair[0]);
    }
    if (tun_pair[1] >= 0) {
        close(tun_pair[1]);
    }
    if (udp_fd >= 0) {
        close(udp_fd);
    }
    if (peer_fd >= 0) {
        close(peer_fd);
    }
    return rc;
}

static int test_udp_pipe_tun_ipv6_guard(void) {
    int tun_pair[2] = {-1, -1};
    int udp_fd = -1;
    int peer_fd = -1;
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tun_pair) != 0) {
        perror("socketpair");
        goto out;
    }
    udp_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    peer_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    if (udp_fd < 0 || peer_fd < 0) {
        perror("socket");
        goto out;
    }

    struct sockaddr_in udp_addr;
    memset(&udp_addr, 0, sizeof(udp_addr));
    udp_addr.sin_family = AF_INET;
    udp_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    udp_addr.sin_port = 0;
    if (bind(udp_fd, (struct sockaddr *)&udp_addr, sizeof(udp_addr)) != 0) {
        perror("bind udp");
        goto out;
    }
    socklen_t udp_len = sizeof(udp_addr);
    if (getsockname(udp_fd, (struct sockaddr *)&udp_addr, &udp_len) != 0) {
        perror("getsockname udp");
        goto out;
    }

    struct sockaddr_in peer_addr;
    memset(&peer_addr, 0, sizeof(peer_addr));
    peer_addr.sin_family = AF_INET;
    peer_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    peer_addr.sin_port = 0;
    if (bind(peer_fd, (struct sockaddr *)&peer_addr, sizeof(peer_addr)) != 0) {
        perror("bind peer");
        goto out;
    }
    socklen_t peer_len = sizeof(peer_addr);
    if (getsockname(peer_fd, (struct sockaddr *)&peer_addr, &peer_len) != 0) {
        perror("getsockname peer");
        goto out;
    }

    const unsigned char allowed_src[16] = {0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2};
    const unsigned char allowed_dst[16] = {0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3};
    const unsigned char bad_src[16] = {0x20, 0x01, 0x0d, 0xb9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2};
    const unsigned char bad_dst[16] = {0x20, 0x01, 0x0d, 0xb9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3};

    struct tapx_ipv6_prefix prefixes[1];
    ipv6_prefix_64(&prefixes[0], allowed_src);

    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tun_pair[0];
    config.udp_fd = udp_fd;
    config.frame_kind = TAPX_FRAME_TUN;
    config.max_frame_size = 2048;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    memcpy(&config.peer_addr, &peer_addr, sizeof(peer_addr));
    config.peer_addr_len = sizeof(peer_addr);
    config.guard.ipv6_prefixes = prefixes;
    config.guard.ipv6_prefix_count = 1;
    config.counters = &counters;

    int start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "udp IPv6 guard worker start")) {
        goto out;
    }

    unsigned char packet[40];
    make_ipv6_packet(packet, bad_src, allowed_dst, 59);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write bad v6 src");
        goto out;
    }
    if (expect_no_data(peer_fd, "disallowed IPv6 source should not reach udp peer")) {
        goto out;
    }

    unsigned char ipv4_packet[20];
    make_ipv4_packet(ipv4_packet, 10, 0, 0, 2, 10, 0, 0, 3);
    if (write(tun_pair[1], ipv4_packet, sizeof(ipv4_packet)) != (ssize_t)sizeof(ipv4_packet)) {
        perror("write unlisted IPv4 family");
        goto out;
    }
    if (expect_no_data(peer_fd, "unlisted IPv4 family should not reach udp peer")) {
        goto out;
    }

    make_ipv6_packet(packet, allowed_src, bad_dst, 59);
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write allowed v6 src");
        goto out;
    }
    unsigned char rx[64];
    ssize_t n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == (ssize_t)sizeof(packet), "allowed IPv6 source reaches udp peer")) {
        goto out;
    }

    make_ipv6_packet(packet, bad_src, bad_dst, 59);
    ssize_t sent = sendto(peer_fd, packet, sizeof(packet), 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send bad v6 dst");
        goto out;
    }
    if (expect_no_data(tun_pair[1], "disallowed IPv6 destination should not reach tun")) {
        goto out;
    }

    make_ipv6_packet(packet, bad_src, allowed_dst, 59);
    sent = sendto(peer_fd, packet, sizeof(packet), 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != (ssize_t)sizeof(packet)) {
        perror("send allowed v6 dst");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)sizeof(packet), "allowed IPv6 destination reaches tun")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "udp IPv6 guard worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 3 && counters.tx_packets == 1 && counters.rx_packets == 1,
               "udp IPv6 guard counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tun_pair[0] >= 0) {
        close(tun_pair[0]);
    }
    if (tun_pair[1] >= 0) {
        close(tun_pair[1]);
    }
    if (udp_fd >= 0) {
        close(udp_fd);
    }
    if (peer_fd >= 0) {
        close(peer_fd);
    }
    return rc;
}

static int test_udp_pipe_tap_mac_arp_guard(void) {
    int tap_pair[2] = {-1, -1};
    int udp_fd = -1;
    int peer_fd = -1;
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tap_pair) != 0) {
        perror("socketpair tap");
        goto out;
    }
    udp_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    peer_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    if (udp_fd < 0 || peer_fd < 0) {
        perror("socket");
        goto out;
    }

    struct sockaddr_in udp_addr;
    memset(&udp_addr, 0, sizeof(udp_addr));
    udp_addr.sin_family = AF_INET;
    udp_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    udp_addr.sin_port = 0;
    if (bind(udp_fd, (struct sockaddr *)&udp_addr, sizeof(udp_addr)) != 0) {
        perror("bind udp");
        goto out;
    }
    socklen_t udp_len = sizeof(udp_addr);
    if (getsockname(udp_fd, (struct sockaddr *)&udp_addr, &udp_len) != 0) {
        perror("getsockname udp");
        goto out;
    }

    struct sockaddr_in peer_addr;
    memset(&peer_addr, 0, sizeof(peer_addr));
    peer_addr.sin_family = AF_INET;
    peer_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    peer_addr.sin_port = 0;
    if (bind(peer_fd, (struct sockaddr *)&peer_addr, sizeof(peer_addr)) != 0) {
        perror("bind peer");
        goto out;
    }
    socklen_t peer_len = sizeof(peer_addr);
    if (getsockname(peer_fd, (struct sockaddr *)&peer_addr, &peer_len) != 0) {
        perror("getsockname peer");
        goto out;
    }

    const unsigned char allowed_mac_bytes[6] = {0x02, 0x00, 0x00, 0x00, 0x00, 0x01};
    const unsigned char other_mac[6] = {0x02, 0x00, 0x00, 0x00, 0x00, 0x02};
    const unsigned char peer_mac[6] = {0x02, 0xaa, 0x00, 0x00, 0x00, 0x01};
    const unsigned char broadcast[6] = {0xff, 0xff, 0xff, 0xff, 0xff, 0xff};
    const unsigned char zero_mac[6] = {0};

    struct tapx_ipv4_prefix prefixes[1];
    prefixes[0].network = 0x0a000000U;
    prefixes[0].mask = 0xffffff00U;
    struct tapx_mac_addr macs[1];
    memcpy(macs[0].bytes, allowed_mac_bytes, 6);

    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tap_pair[0];
    config.udp_fd = udp_fd;
    config.frame_kind = TAPX_FRAME_TAP;
    config.max_frame_size = 2048;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    memcpy(&config.peer_addr, &peer_addr, sizeof(peer_addr));
    config.peer_addr_len = sizeof(peer_addr);
    config.guard.ipv4_prefixes = prefixes;
    config.guard.ipv4_prefix_count = 1;
    config.guard.macs = macs;
    config.guard.mac_count = 1;
    config.counters = &counters;

    int start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "tap guard worker start")) {
        goto out;
    }

    unsigned char frame[64];
    make_tap_ipv4_frame(frame, peer_mac, other_mac, 10, 0, 0, 2, 10, 0, 0, 3);
    if (write(tap_pair[1], frame, 34) != 34) {
        perror("write bad src mac");
        goto out;
    }
    if (expect_no_data(peer_fd, "disallowed TAP source MAC should not reach udp peer")) {
        goto out;
    }

    make_tap_ipv4_frame(frame, peer_mac, allowed_mac_bytes, 10, 0, 1, 2, 10, 0, 0, 3);
    if (write(tap_pair[1], frame, 34) != 34) {
        perror("write bad src ip");
        goto out;
    }
    if (expect_no_data(peer_fd, "disallowed TAP source IP should not reach udp peer")) {
        goto out;
    }

    make_tap_ipv4_frame(frame, peer_mac, allowed_mac_bytes, 10, 0, 0, 2, 10, 0, 1, 3);
    if (write(tap_pair[1], frame, 34) != 34) {
        perror("write allowed tap frame");
        goto out;
    }
    unsigned char rx[64];
    ssize_t n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == 34, "allowed TAP frame reaches udp peer")) {
        goto out;
    }

    make_tap_vlan_ipv4_frame(frame, peer_mac, allowed_mac_bytes, 0);
    if (write(tap_pair[1], frame, 38) != 38) {
        perror("write allowed VLAN frame");
        goto out;
    }
    n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == 38, "allowed VLAN frame reaches udp peer")) {
        goto out;
    }

    make_tap_vlan_ipv4_frame(frame, peer_mac, allowed_mac_bytes, 1);
    if (write(tap_pair[1], frame, 38) != 38) {
        perror("write disallowed VLAN frame");
        goto out;
    }
    if (expect_no_data(peer_fd, "VLAN IPv4 source should not bypass guard")) {
        goto out;
    }

    make_tap_pppoe_ipv4_frame(frame, peer_mac, allowed_mac_bytes, 0);
    if (write(tap_pair[1], frame, 42) != 42) {
        perror("write allowed PPPoE frame");
        goto out;
    }
    n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == 42, "allowed PPPoE frame reaches udp peer")) {
        goto out;
    }

    make_tap_pppoe_ipv4_frame(frame, peer_mac, allowed_mac_bytes, 1);
    if (write(tap_pair[1], frame, 42) != 42) {
        perror("write disallowed PPPoE frame");
        goto out;
    }
    if (expect_no_data(peer_fd, "PPPoE IPv4 source should not bypass guard")) {
        goto out;
    }

    make_tap_ipv4_frame(frame, other_mac, peer_mac, 10, 0, 1, 3, 10, 0, 0, 2);
    ssize_t sent = sendto(peer_fd, frame, 34, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != 34) {
        perror("send bad dst mac");
        goto out;
    }
    if (expect_no_data(tap_pair[1], "disallowed TAP destination MAC should not reach tap")) {
        goto out;
    }

    make_tap_arp_frame(frame, broadcast, peer_mac, peer_mac, 10, 0, 1, 9,
                       zero_mac, 10, 0, 1, 2);
    sent = sendto(peer_fd, frame, 42, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != 42) {
        perror("send bad arp target");
        goto out;
    }
    if (expect_no_data(tap_pair[1], "ARP for disallowed target IP should not reach tap")) {
        goto out;
    }

    make_tap_arp_frame(frame, broadcast, peer_mac, peer_mac, 10, 0, 1, 9,
                       zero_mac, 10, 0, 0, 2);
    sent = sendto(peer_fd, frame, 42, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != 42) {
        perror("send allowed arp target");
        goto out;
    }
    n = read(tap_pair[1], rx, sizeof(rx));
    if (expect(n == 42, "ARP for allowed target IP reaches tap")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "tap guard worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 6 && counters.tx_packets == 3 && counters.rx_packets == 1,
               "tap guard counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tap_pair[0] >= 0) {
        close(tap_pair[0]);
    }
    if (tap_pair[1] >= 0) {
        close(tap_pair[1]);
    }
    if (udp_fd >= 0) {
        close(udp_fd);
    }
    if (peer_fd >= 0) {
        close(peer_fd);
    }
    return rc;
}

static int test_udp_pipe_tap_ipv6_nd_guard(void) {
    int tap_pair[2] = {-1, -1};
    int udp_fd = -1;
    int peer_fd = -1;
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tap_pair) != 0) {
        perror("socketpair tap");
        goto out;
    }
    udp_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    peer_fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC, 0);
    if (udp_fd < 0 || peer_fd < 0) {
        perror("socket");
        goto out;
    }

    struct sockaddr_in udp_addr;
    memset(&udp_addr, 0, sizeof(udp_addr));
    udp_addr.sin_family = AF_INET;
    udp_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    udp_addr.sin_port = 0;
    if (bind(udp_fd, (struct sockaddr *)&udp_addr, sizeof(udp_addr)) != 0) {
        perror("bind udp");
        goto out;
    }
    socklen_t udp_len = sizeof(udp_addr);
    if (getsockname(udp_fd, (struct sockaddr *)&udp_addr, &udp_len) != 0) {
        perror("getsockname udp");
        goto out;
    }

    struct sockaddr_in peer_addr;
    memset(&peer_addr, 0, sizeof(peer_addr));
    peer_addr.sin_family = AF_INET;
    peer_addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    peer_addr.sin_port = 0;
    if (bind(peer_fd, (struct sockaddr *)&peer_addr, sizeof(peer_addr)) != 0) {
        perror("bind peer");
        goto out;
    }
    socklen_t peer_len = sizeof(peer_addr);
    if (getsockname(peer_fd, (struct sockaddr *)&peer_addr, &peer_len) != 0) {
        perror("getsockname peer");
        goto out;
    }

    const unsigned char allowed_mac[6] = {0x02, 0, 0, 0, 0, 1};
    const unsigned char peer_mac[6] = {0x02, 0xaa, 0, 0, 0, 1};
    const unsigned char other_mac[6] = {0x02, 0, 0, 0, 0, 2};
    const unsigned char multicast_mac[6] = {0x33, 0x33, 0xff, 0x00, 0x00, 0x02};
    const unsigned char allowed_ip[16] = {0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2};
    const unsigned char peer_ip[16] = {0x20, 0x01, 0x0d, 0xba, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9};
    const unsigned char bad_ip[16] = {0x20, 0x01, 0x0d, 0xb9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2};
    const unsigned char solicited[16] = {0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0xff, 0, 0, 2};

    struct tapx_ipv6_prefix prefixes[1];
    ipv6_prefix_64(&prefixes[0], allowed_ip);
    struct tapx_mac_addr macs[1];
    memcpy(macs[0].bytes, allowed_mac, 6);

    struct tapx_udp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tap_pair[0];
    config.udp_fd = udp_fd;
    config.frame_kind = TAPX_FRAME_TAP;
    config.max_frame_size = 2048;
    config.peer_mode = TAPX_UDP_PEER_FIXED;
    memcpy(&config.peer_addr, &peer_addr, sizeof(peer_addr));
    config.peer_addr_len = sizeof(peer_addr);
    config.guard.ipv6_prefixes = prefixes;
    config.guard.ipv6_prefix_count = 1;
    config.guard.macs = macs;
    config.guard.mac_count = 1;
    config.counters = &counters;

    int start_rc = tapx_udp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "tap IPv6 guard worker start")) {
        goto out;
    }

    unsigned char frame[86];
    make_tap_ipv6_frame(frame, peer_mac, allowed_mac, bad_ip, peer_ip);
    if (write(tap_pair[1], frame, 54) != 54) {
        perror("write bad v6 source");
        goto out;
    }
    if (expect_no_data(peer_fd, "disallowed TAP IPv6 source should not reach udp peer")) {
        goto out;
    }

    make_tap_ipv6_frame(frame, peer_mac, allowed_mac, allowed_ip, peer_ip);
    if (write(tap_pair[1], frame, 54) != 54) {
        perror("write allowed v6 source");
        goto out;
    }
    unsigned char rx[128];
    ssize_t n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == 54, "allowed TAP IPv6 source reaches udp peer")) {
        goto out;
    }

    make_tap_ipv6_frame(frame, allowed_mac, peer_mac, peer_ip, bad_ip);
    ssize_t sent = sendto(peer_fd, frame, 54, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != 54) {
        perror("send bad v6 destination");
        goto out;
    }
    if (expect_no_data(tap_pair[1], "disallowed TAP IPv6 destination should not reach tap")) {
        goto out;
    }

    make_tap_nd_ns_frame(frame, multicast_mac, peer_mac, peer_ip, solicited, bad_ip, peer_mac);
    sent = sendto(peer_fd, frame, 86, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != 86) {
        perror("send bad nd target");
        goto out;
    }
    if (expect_no_data(tap_pair[1], "ND for disallowed target should not reach tap")) {
        goto out;
    }

    make_tap_nd_ns_frame(frame, multicast_mac, peer_mac, peer_ip, solicited, allowed_ip, peer_mac);
    sent = sendto(peer_fd, frame, 86, 0, (struct sockaddr *)&udp_addr, udp_len);
    if (sent != 86) {
        perror("send allowed nd target");
        goto out;
    }
    n = read(tap_pair[1], rx, sizeof(rx));
    if (expect(n == 86, "ND for allowed target reaches tap")) {
        goto out;
    }

    make_tap_ipv6_frame(frame, other_mac, allowed_mac, allowed_ip, peer_ip);
    if (write(tap_pair[1], frame, 54) != 54) {
        perror("write allowed v6 wrong src mac");
        goto out;
    }
    n = recv(peer_fd, rx, sizeof(rx), 0);
    if (expect(n == 54, "TAP IPv6 ignores remote destination MAC on outbound")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "tap IPv6 guard worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 3 && counters.tx_packets == 2 && counters.rx_packets == 1,
               "tap IPv6 guard counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tap_pair[0] >= 0) {
        close(tap_pair[0]);
    }
    if (tap_pair[1] >= 0) {
        close(tap_pair[1]);
    }
    if (udp_fd >= 0) {
        close(udp_fd);
    }
    if (peer_fd >= 0) {
        close(peer_fd);
    }
    return rc;
}

static uint16_t read_be16(const unsigned char *p) {
    return (uint16_t)(((uint16_t)p[0] << 8) | (uint16_t)p[1]);
}

static void write_be16(unsigned char *p, uint16_t value) {
    p[0] = (unsigned char)(value >> 8);
    p[1] = (unsigned char)(value & 0xff);
}

static int test_tcp_pipe_rejects_bad_config(void) {
    struct tapx_worker *worker = NULL;
    struct tapx_tcp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = -1;
    config.tcp_fd = -1;
    config.frame_kind = TAPX_FRAME_TUN;
    config.length_mode = TAPX_TCP_LENGTH_UINT16;
    int rc = tapx_tcp_pipe_start(&config, &worker);
    if (expect(rc < 0, "invalid tcp config should fail")) {
        return 1;
    }
    return 0;
}

static int test_tcp_pipe_starts_and_stops(void) {
    int tun_pair[2] = {-1, -1};
    int tcp_pair[2] = {-1, -1};
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tun_pair) != 0) {
        perror("socketpair tun");
        goto out;
    }
    if (socketpair(AF_UNIX, SOCK_STREAM, 0, tcp_pair) != 0) {
        perror("socketpair tcp");
        goto out;
    }

    struct tapx_tcp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tun_pair[0];
    config.tcp_fd = tcp_pair[0];
    config.frame_kind = TAPX_FRAME_TUN;
    config.max_frame_size = 2048;
    config.length_mode = TAPX_TCP_LENGTH_UINT16;
    config.counters = &counters;

    int start_rc = tapx_tcp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "tcp worker start")) {
        goto out;
    }

    const unsigned char packet[] = {0x45, 0x00, 0x00, 0x14};
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write tcp tun peer");
        goto out;
    }
    unsigned char rx[32];
    ssize_t n = read(tcp_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)(sizeof(packet) + 2), "tcp framed receive")) {
        goto out;
    }
    if (expect(read_be16(rx) == sizeof(packet), "tcp length header")) {
        goto out;
    }
    if (expect(memcmp(rx + 2, packet, sizeof(packet)) == 0, "tcp payload")) {
        goto out;
    }

    unsigned char incoming[6];
    write_be16(incoming, 4);
    incoming[2] = 0x60;
    incoming[3] = 0x00;
    incoming[4] = 0x00;
    incoming[5] = 0x00;
    if (write(tcp_pair[1], incoming, sizeof(incoming)) != (ssize_t)sizeof(incoming)) {
        perror("write tcp peer");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == 4, "tun receives tcp frame")) {
        goto out;
    }
    if (expect(memcmp(rx, incoming + 2, 4) == 0, "tun tcp payload")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "tcp worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.tx_packets == 1 && counters.rx_packets == 1,
               "tcp counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tun_pair[0] >= 0) {
        close(tun_pair[0]);
    }
    if (tun_pair[1] >= 0) {
        close(tun_pair[1]);
    }
    if (tcp_pair[0] >= 0) {
        close(tcp_pair[0]);
    }
    if (tcp_pair[1] >= 0) {
        close(tcp_pair[1]);
    }
    return rc;
}

static int test_tcp_pipe_vkey_header(void) {
    int tun_pair[2] = {-1, -1};
    int tcp_pair[2] = {-1, -1};
    int rc = 1;
    struct tapx_worker *worker = NULL;
    struct tapx_fastpath_counters counters;
    tapx_fastpath_counters_reset(&counters);

    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, tun_pair) != 0) {
        perror("socketpair tun");
        goto out;
    }
    if (socketpair(AF_UNIX, SOCK_STREAM, 0, tcp_pair) != 0) {
        perror("socketpair tcp");
        goto out;
    }

    const unsigned char key[] = "tcp-vk";
    struct tapx_tcp_pipe_config config;
    memset(&config, 0, sizeof(config));
    config.tun_fd = tun_pair[0];
    config.tcp_fd = tcp_pair[0];
    config.frame_kind = TAPX_FRAME_TUN;
    config.max_frame_size = 2048;
    config.length_mode = TAPX_TCP_LENGTH_UINT16;
    config.vkey.value = key;
    config.vkey.value_len = sizeof(key) - 1;
    config.counters = &counters;

    int start_rc = tapx_tcp_pipe_start(&config, &worker);
    if (expect(start_rc == 0 && worker != NULL, "tcp vkey worker start")) {
        goto out;
    }

    const unsigned char packet[] = {0x45, 0x00, 0x00, 0x14};
    if (write(tun_pair[1], packet, sizeof(packet)) != (ssize_t)sizeof(packet)) {
        perror("write tcp vkey tun");
        goto out;
    }
    unsigned char rx[128];
    ssize_t n = read(tcp_pair[1], rx, sizeof(rx));
    size_t vkey_len = 8 + sizeof(key) - 1;
    if (expect(n == (ssize_t)(2 + vkey_len + sizeof(packet)), "tcp vkey wire length")) {
        goto out;
    }
    if (expect(read_be16(rx) == vkey_len + sizeof(packet), "tcp vkey length header")) {
        goto out;
    }
    if (expect(memcmp(rx + 2, "TXV1", 4) == 0 &&
               memcmp(rx + 2 + 8, key, sizeof(key) - 1) == 0 &&
               memcmp(rx + 2 + vkey_len, packet, sizeof(packet)) == 0,
               "tcp vkey wire payload")) {
        goto out;
    }

    unsigned char bad[6];
    write_be16(bad, sizeof(packet));
    memcpy(bad + 2, packet, sizeof(packet));
    if (write(tcp_pair[1], bad, sizeof(bad)) != (ssize_t)sizeof(bad)) {
        perror("write tcp missing vkey");
        goto out;
    }
    if (expect_no_data(tun_pair[1], "tcp missing vkey should not reach tun")) {
        goto out;
    }

    unsigned char wire[128];
    size_t wire_payload_len = make_vkey_payload(wire + 2, key, sizeof(key) - 1, packet, sizeof(packet));
    write_be16(wire, (uint16_t)wire_payload_len);
    if (write(tcp_pair[1], wire, wire_payload_len + 2) != (ssize_t)(wire_payload_len + 2)) {
        perror("write tcp vkey payload");
        goto out;
    }
    n = read(tun_pair[1], rx, sizeof(rx));
    if (expect(n == (ssize_t)sizeof(packet), "tcp vkey payload reaches tun")) {
        goto out;
    }
    if (expect(memcmp(rx, packet, sizeof(packet)) == 0, "tcp stripped vkey payload")) {
        goto out;
    }

    if (expect(tapx_worker_stop(worker) == 0, "tcp vkey worker stop")) {
        worker = NULL;
        goto out;
    }
    worker = NULL;
    if (expect(counters.drops_guard == 1 && counters.tx_packets == 1 && counters.rx_packets == 1,
               "tcp vkey counters")) {
        goto out;
    }
    rc = 0;

out:
    if (worker != NULL) {
        (void)tapx_worker_stop(worker);
    }
    if (tun_pair[0] >= 0) {
        close(tun_pair[0]);
    }
    if (tun_pair[1] >= 0) {
        close(tun_pair[1]);
    }
    if (tcp_pair[0] >= 0) {
        close(tcp_pair[0]);
    }
    if (tcp_pair[1] >= 0) {
        close(tcp_pair[1]);
    }
    return rc;
}

int main(void) {
    if (test_abi_and_counters() != 0) {
        return 1;
    }
    if (test_udp_pipe_rejects_bad_config() != 0) {
        return 1;
    }
    if (test_udp_pipe_starts_and_stops() != 0) {
        return 1;
    }
    if (test_udp_pipe_vkey_header() != 0) {
        return 1;
    }
    if (test_udp_pipe_tun_ipv4_guard() != 0) {
        return 1;
    }
    if (test_udp_pipe_tun_ipv6_guard() != 0) {
        return 1;
    }
    if (test_udp_pipe_tap_mac_arp_guard() != 0) {
        return 1;
    }
    if (test_udp_pipe_tap_ipv6_nd_guard() != 0) {
        return 1;
    }
    if (test_tcp_pipe_rejects_bad_config() != 0) {
        return 1;
    }
    if (test_tcp_pipe_starts_and_stops() != 0) {
        return 1;
    }
    if (test_tcp_pipe_vkey_header() != 0) {
        return 1;
    }
    return 0;
}
