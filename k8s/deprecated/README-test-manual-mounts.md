


```bash
# Test the same mount that worked before
kubectl run nfs-debug --image=busybox --rm -it -- /bin/sh

# In the debug pod:
mkdir -p /mnt/test
mount -t nfs -o nfsvers=4.1,proto=tcp 10.109.83.84:/globeco-shared /mnt/test
# If this fails, try:
mount -t nfs -o nfsvers=4,proto=tcp 10.109.83.84:/globeco-shared /mnt/test
# Or try mounting root first:
mount -t nfs -o nfsvers=4,proto=tcp 10.109.83.84:/ /mnt/test
ls -la /mnt/test/
```


