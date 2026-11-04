import * as podman from "@geoffsee/podman";
import * as pulumi from "@pulumi/pulumi";

const labels = {
    "managed-by": "pulumi",
    "pulumi-playground": "true",
};

const demoVolume = new podman.volumes.Volume("demoVolume", {
    name: "pulumi-podman-playground-data",
    labels,
});

const demoNetwork = new podman.networks.Network("demoNetwork", {
    name: "pulumi-podman-playground-net",
    labels,
});

const busybox = new podman.images.Image("busybox", {
    reference: "docker.io/library/busybox:latest",
    policy: "missing",
    quiet: true,
});

const containerNetworks = demoNetwork.name.apply((net) =>
    Object.fromEntries([[net, {}]]),
);

const ticker = new podman.containers.Container(
    "ticker",
    {
        name: "pulumi-podman-playground-ticker",
        image: busybox.reference,
        command: ["sh", "-c", "while true; do date; sleep 5; done"],
        labels,
        networks: containerNetworks,
    },
    { dependsOn: [busybox, demoNetwork] },
);

const logger = new podman.containers.Container(
    "logger",
    {
        name: "pulumi-podman-playground-logger",
        image: busybox.reference,
        command: [
            "sh",
            "-c",
            'while true; do echo "heartbeat" >> /data/heartbeat; sleep 10; done',
        ],
        labels,
        networks: containerNetworks,
        volumes: demoVolume.name.apply((vol) => [{ name: vol, dest: "/data" }]),
    },
    { dependsOn: [busybox, demoNetwork, demoVolume] },
);

const sleeper = new podman.containers.Container(
    "sleeper",
    {
        name: "pulumi-podman-playground-sleeper",
        image: busybox.reference,
        command: ["sleep", "infinity"],
        labels,
        networks: containerNetworks,
    },
    { dependsOn: [busybox, demoNetwork] },
);

export const volumeName = demoVolume.name;
export const networkName = demoNetwork.name;
export const busyboxReference = busybox.reference;
export const containerNames = pulumi.all([
    ticker.name,
    logger.name,
    sleeper.name,
]);
