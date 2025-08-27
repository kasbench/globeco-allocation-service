# Remove the problematic export first (if necessary)
kubectl -n rook-ceph exec -it deploy/rook-ceph-tools -- rados -p .nfs -N globeco-nfs rm export-2

# Create a new export with ID 2 and root path
kubectl -n rook-ceph exec -it deploy/rook-ceph-tools -- bash -c '
cat > /tmp/export-3 << EOF
EXPORT {
    Export_Id = 3;
    Path = "/";
    Pseudo = "/globeco-shared-nfs";
    Protocols = 4;
    Transports = TCP;
    Access_Type = RW;
    Squash = no_root_squash;
    FSAL {
        Name = CEPH;
        Filesystem = "globeco";
        User_Id = "admin";
    }
    CLIENT {
        Clients = *;
        Access_Type = RW;
        Squash = no_root_squash;
    }
}
EOF

# Store the new export
rados -p .nfs -N globeco-nfs put export-3 /tmp/export-3

echo "Created new export with ID 3:"
cat /tmp/export-3
'

# Reload configuration
kubectl -n rook-ceph exec deploy/rook-ceph-nfs-globeco-nfs-a -c nfs-ganesha -- kill -HUP 1

# Check logs
sleep 5
kubectl logs -n rook-ceph deploy/rook-ceph-nfs-globeco-nfs-a -c nfs-ganesha --tail=10