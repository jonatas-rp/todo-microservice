# 0010. BoltDB file on the Windows host

- Status: Accepted
- Date: 2026-11-25

## Context

Deleting an overlay deletes its Namespace, and with it the PersistentVolumeClaim. The database file has to still be on disk afterward. Docker Desktop's kubelet mounts host paths from the Windows drive. A path under the Ubuntu home directory is not a directory in that mount namespace.

## Decision

Each environment has a static PersistentVolume, class `manual`, reclaim `Retain`, bound by `volumeName` on the claim `todo-store-data`. The hostPath is `/run/desktop/mnt/host/c/Users/jorp5/arancia-todo-data/development` or `.../production`, created if missing. The store container mounts the claim at `/data`.

WSL sees that Windows directory through the symlink `todo-service/data`. The symlink is gitignored.

## Consequences

- `kubectl delete -k` on an overlay removes the namespace and both workloads. The `todos.db` file stays in the host directory.
- Development and production use different directories, so they do not share one BoltDB file.
- The claim is ReadWriteOnce. The store Deployment rolls with `maxSurge: 1` and `maxUnavailable: 0`, so the new pod mounts the volume after the old pod releases it.
