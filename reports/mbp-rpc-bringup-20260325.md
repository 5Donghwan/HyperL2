# MBP Dedicated RPC Bring-up Report (2026-03-25)

## Goal

Bring up a dedicated Kaia RPC node on the accessible MacBook Pro (`ssh lab`) so HyperL2 can stop relying on the shared public zkrypto RPC endpoint.

## What Completed Successfully

1. Verified lab bootstrap files exist locally:
   - `/Users/5d0ng/dev/HyperL2/devinfo/testnet-genesis.json`
   - `/Users/5d0ng/dev/HyperL2/devinfo/static-nodes.json`
2. Confirmed the chain ID in the provided genesis is `113230`.
3. Confirmed the provided peer list contains `cn` / `pn` peers, so the MBP should run an Endpoint Node (`ken`) instead of a service-chain endpoint by default.
4. Generated and uploaded the MBP bootstrap bundle to:
   - `/Users/odonghwan/dev/kaia-service-rpc/hyperl2-mbp-rpc-bundle`
5. Installed Go `1.25.3` on the MBP under:
   - `/Users/odonghwan/dev/go-bootstrap/go`
6. Cloned `kaiachain/kaia` and built `ken` from source at tag `v2.2.2`.
7. Initialized the MBP datadir with the provided genesis:
   - `/Users/odonghwan/dev/kaia-service-rpc/data`
8. Started `ken` on the MBP with:
   - `networkid=113230`
   - `rpc=8551`
   - `ws=8552`
   - `datadir=/Users/odonghwan/dev/kaia-service-rpc/data`
9. Verified the MBP RPC is reachable from the local machine:
   - `http://100.106.151.80:8551`

## What Is Working Right Now

- `rpc_modules` responds correctly from the MBP RPC
- `eth_chainId` returns `0x1ba4e`
- HTTP RPC is reachable from the local HyperL2 host
- The MBP EN process stays up

## What Is Not Working Yet

The MBP node is not syncing.

Observed symptoms:

- `net_peerCount = 0`
- `eth_blockNumber = 0`
- latest block remains the genesis block

## Root Cause

The peer endpoints in the provided `static-nodes.json` are not reachable from the MBP.

Direct connectivity tests from the MBP:

- `192.168.20.5:32723` -> timeout
- `192.168.20.5:32724` -> timeout
- `192.168.40.10:32723` -> timeout

The MBP network view:

- primary LAN: `192.168.0.6`
- Tailscale: `100.106.151.80`

So the current MBP does not have a route to the `192.168.20.0/24` and `192.168.40.0/24` peer subnets listed in the bootstrap file.

## Practical Meaning

The dedicated RPC bring-up is mechanically correct:

- bootstrap bundle is valid
- `ken` build is valid
- genesis init is valid
- RPC exposure is valid

But without reachable peers, this MBP EN cannot synchronize the lab chain, so it cannot yet replace the shared zkrypto RPC for HyperL2 measurements.

## What We Need From The Lab Side

One of the following:

1. Reachable peer addresses for the MBP
   - updated `static-nodes.json` using addresses the MBP can route to
2. Network access to the existing peer subnets
   - routing / VPN / firewall changes so the MBP can reach `192.168.20.*` and `192.168.40.*`
3. A nodekey if the lab requires fixed node identity / allowlisting

## Recommended Next Step

Ask the lab for a peer list that is reachable from the MBP's current network context, or for the network path needed to reach the existing private peer IPs.
