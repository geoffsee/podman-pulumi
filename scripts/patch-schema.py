#!/usr/bin/env python3
"""Patch provider schema for SDK-friendly network map types."""
from __future__ import annotations

import json
import sys
from pathlib import Path

PER_NETWORK = {
    "type": "object",
    "properties": {
        "aliases": {
            "type": "array",
            "items": {"type": "string"},
            "description": "DNS aliases for this container on the network.",
        },
        "interfaceName": {
            "type": "string",
            "description": "Network interface name inside the container.",
        },
        "options": {
            "type": "object",
            "additionalProperties": {"type": "string"},
            "description": "Driver-specific options for this network attachment.",
        },
        "staticIps": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Static IP addresses on this network.",
        },
        "staticMac": {
            "type": "string",
            "description": "Static MAC address on this network.",
        },
    },
    "description": "Per-network options when attaching a container or pod to a network.",
}

NETWORKS_MAP = {
    "type": "object",
    "additionalProperties": {"$ref": "#/types/podman:index:PerNetworkOptions"},
    "description": "Map of network names or IDs to per-network options.",
}


def patch_networks(props: dict) -> None:
    if "networks" in props:
        props["networks"] = {**NETWORKS_MAP, "description": props["networks"].get("description", NETWORKS_MAP["description"])}


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else "schema.json")
    schema = json.loads(path.read_text())
    types = schema.setdefault("types", {})
    types["podman:index:PerNetworkOptions"] = PER_NETWORK

    for token in (
        "podman:containers:Container",
        "podman:pods:Pod",
    ):
        res = schema.get("resources", {}).get(token, {})
        for key in ("inputProperties", "properties"):
            if key in res:
                patch_networks(res[key])

    path.write_text(json.dumps(schema, indent=2) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
