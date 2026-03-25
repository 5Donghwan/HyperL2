# MBP Dedicated RPC Setup

## Goal

Run a dedicated Kaia RPC node on an accessible MacBook Pro so HyperL2 stops depending on the shared public zkrypto RPC path.

## What We Want

The MBP should not be a simple HTTP proxy. It should be a real Kaia node that synchronizes the lab chain and exposes its own HTTP / WebSocket RPC.

Why:

- A plain proxy would still inherit the shared upstream's receipt-visibility lag.
- A real Kaia node gives us a private observation point for txpool, blocks, receipts, and head progression.

## Recommended Mode

Use the MBP as a dedicated Service Chain Endpoint Node (`SEN`) or equivalent RPC-only node for the lab chain.

This is an inference from the official Kaia docs:
- Service Chain is explicitly positioned for high TPS, low fees, privacy, and local/private testing.
- SCN, SPN, and SEN packages are distributed together, and their config properties are documented as sharing the same structure.

## Required Inputs From the Lab Chain

Before the MBP can join the chain, we still need chain bootstrap material from the lab side:

- `genesis.json`
- `static-nodes.json`
- `NETWORK_ID` / chain ID
- optional dedicated `nodekey` for the MBP RPC node
- open P2P access from the MBP to existing lab peers

Without these, we can only prepare the node, not join the chain.

## Files Added In This Repo

- env template: `/Users/5d0ng/dev/HyperL2/configs/mbp-rpc/service-chain.env.example`
- config renderer: `/Users/5d0ng/dev/HyperL2/scripts/render-kaia-service-rpc-conf.sh`
- staging helper: `/Users/5d0ng/dev/HyperL2/scripts/stage-mbp-rpc-bundle.sh`
- upload helper: `/Users/5d0ng/dev/HyperL2/scripts/push-mbp-rpc-bundle.sh`
- health check: `/Users/5d0ng/dev/HyperL2/scripts/check-kaia-rpc.sh`

## Step-by-Step

### 1. Download Kaia packages on the MBP

Use the official Kaia download page and fetch the latest stable build that includes service-chain binaries.

Needed binaries:

- `ksend` / `ksend.conf` if the lab network expects a Service Chain Endpoint Node
- optionally `homi` only if we later build our own private chain

Docs:
- Kaia node downloads page
- Service Chain install guide

### 2. Extract packages on the MBP

Example layout from the docs:

- `bin/ksend`
- `bin/ksendd`
- `conf/ksend.conf`

### 3. Prepare bootstrap files

Copy onto the MBP:

- `genesis.json`
- `static-nodes.json`
- optional `nodekey`

Suggested destination:

- `${KAIA_DATA_DIR}/genesis.json`
- `${KAIA_DATA_DIR}/static-nodes.json`
- `${KAIA_DATA_DIR}/klay/nodekey`

### 4. Stage the MBP RPC bundle locally

```bash
scripts/stage-mbp-rpc-bundle.sh \
  /Users/5d0ng/dev/HyperL2/configs/mbp-rpc/service-chain.env.example \
  /Users/5d0ng/dev/HyperL2/build/mbp-rpc/staged
```

This produces:

- `conf/ksend.conf`
- `bootstrap/genesis.json`
- `bootstrap/static-nodes.json`
- `bootstrap/nodekey` if one was supplied

Then upload the staged bundle to the MBP and place the rendered config at the extracted package's `conf/ksend.conf` path.

If SSH access to the MBP is already available, you can upload the staged bundle directly:

```bash
scripts/push-mbp-rpc-bundle.sh \
  /Users/5d0ng/dev/HyperL2/configs/mbp-rpc/service-chain.env.example \
  /Users/5d0ng/dev/HyperL2/build/mbp-rpc/staged
```

### 5. Start the RPC node on the MBP

Example, on the MBP:

```bash
cd "$KAIA_INSTALL_DIR"
./bin/ksendd start
```

### 6. Validate the MBP RPC

From this repo:

```bash
scripts/check-kaia-rpc.sh http://<MBP-IP>:8551
```

Expected checks:

- `rpc_modules` responds
- `eth_chainId` matches the lab chain
- `eth_blockNumber` moves forward
- latest block timestamps keep advancing

### What the MBP bundle still depends on

The bundle prepares everything we can safely automate from this repo, but the MBP still needs:

- a real Kaia package extracted on disk
- a genesis initialization step on the MBP data directory
- P2P reachability to the lab chain peers listed in `static-nodes.json`

### 7. Point HyperL2 at the MBP RPC

Once the MBP node is healthy:

```bash
export KAIA_RPC_URL="http://<MBP-IP>:8551"
scripts/run-kaia-16p6s-repeat.sh
```

or for the dashboard:

```bash
export KAIA_RPC_URL="http://<MBP-IP>:8551"
scripts/run-kaia-16p6s-dashboard.sh
```

## What I Expect To Improve

If the MBP is a real synchronized Kaia node, we should get:

- lower variance in `TransactionReceipt` visibility
- more stable block/header observation
- better separation between proposer behavior and public gateway lag

This may or may not improve actual block inclusion, but it should remove a major source of measurement noise.

## Important Limitation

If the lab chain's proposer itself delays inclusion, a dedicated RPC node will not fix proposer latency.
It will only remove the extra uncertainty from the shared public RPC path.

## Notes From Official Docs

- Endpoint Nodes are the interface for sending transactions and querying chain state.
- HTTP / WS RPC modules must be explicitly enabled with `RPC_ENABLE`, `RPC_API`, `WS_ENABLE`, and `WS_API`.
- Service-chain configuration files for SCN, SPN, and SEN share the same property structure.
- Initializing the node data directory with the correct `genesis.json` is required before startup.
- `static-nodes.json` and `nodekey` are part of the standard service-chain bootstrap flow.

## Official References

- Kaia Endpoint Node overview: https://docs.kaia.io/nodes/endpoint-node/
- Kaia JSON-RPC enablement: https://docs.kaia.io/nodes/endpoint-node/json-rpc-apis/
- Kaia node downloads: https://docs.kaia.io/nodes/downloads/
- Kaia Service Chain install guide: https://docs.kaia.io/nodes/service-chain/install-service-chain/
- Kaia Service Chain configuration files: https://docs.kaia.io/nodes/service-chain/configure/configuration-files/
- Kaia 4-node Service Chain quick start: https://docs.kaia.io/nodes/service-chain/quick-start/4nodes-setup-guide/
