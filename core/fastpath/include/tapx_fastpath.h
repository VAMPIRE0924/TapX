#ifndef TAPX_FASTPATH_H
#define TAPX_FASTPATH_H

#include <stddef.h>
#include <stdint.h>
#include <sys/socket.h>

#ifdef __cplusplus
extern "C" {
#endif

#define TAPX_FASTPATH_ABI_VERSION 8u

enum tapx_frame_kind {
    TAPX_FRAME_TUN = 1,
    TAPX_FRAME_TAP = 2
};

enum tapx_udp_peer_mode {
    TAPX_UDP_PEER_ANY = 0,
    TAPX_UDP_PEER_FIXED = 1,
    TAPX_UDP_PEER_LEARN = 2
};

enum tapx_tcp_length_mode {
    TAPX_TCP_LENGTH_UINT16 = 16,
    TAPX_TCP_LENGTH_UINT32 = 32
};

struct tapx_fastpath_counters {
    uint64_t rx_packets;
    uint64_t tx_packets;
    uint64_t rx_bytes;
    uint64_t tx_bytes;
    uint64_t drops_guard;
    uint64_t drops_io;
};

struct tapx_ipv4_prefix {
    uint32_t network;
    uint32_t mask;
};

struct tapx_ipv6_prefix {
    uint8_t network[16];
    uint8_t mask[16];
};

struct tapx_mac_addr {
    uint8_t bytes[6];
};

struct tapx_address_guard {
    const struct tapx_ipv4_prefix *ipv4_prefixes;
    size_t ipv4_prefix_count;
    const struct tapx_ipv6_prefix *ipv6_prefixes;
    size_t ipv6_prefix_count;
    const struct tapx_mac_addr *macs;
    size_t mac_count;
};

struct tapx_vkey_guard {
    const uint8_t *value;
    size_t value_len;
};

struct tapx_udp_pipe_config {
    int tun_fd;
    int udp_fd;
    uint32_t frame_kind;
    uint32_t max_frame_size;
    uint32_t max_datagram_payload;
    uint32_t peer_mode;
    uint32_t address_guard_remote;
    uint64_t device_to_network_rate_bps;
    uint64_t network_to_device_rate_bps;
    struct sockaddr_storage peer_addr;
    socklen_t peer_addr_len;
    struct tapx_address_guard guard;
    struct tapx_vkey_guard vkey;
    struct tapx_fastpath_counters *counters;
};

struct tapx_tcp_pipe_config {
    int tun_fd;
    int tcp_fd;
    uint32_t frame_kind;
    uint32_t max_frame_size;
    uint32_t length_mode;
    uint32_t address_guard_remote;
    uint64_t device_to_network_rate_bps;
    uint64_t network_to_device_rate_bps;
    struct tapx_address_guard guard;
    struct tapx_vkey_guard vkey;
    struct tapx_fastpath_counters *counters;
};

struct tapx_worker;

uint32_t tapx_fastpath_abi_version(void);
void tapx_fastpath_counters_reset(struct tapx_fastpath_counters *counters);
void tapx_fastpath_counters_snapshot(const struct tapx_fastpath_counters *counters,
                                     struct tapx_fastpath_counters *snapshot);
int tapx_udp_pipe_start(const struct tapx_udp_pipe_config *config, struct tapx_worker **worker);
int tapx_tcp_pipe_start(const struct tapx_tcp_pipe_config *config, struct tapx_worker **worker);
int tapx_worker_stop(struct tapx_worker *worker);

#ifdef __cplusplus
}
#endif

#endif
