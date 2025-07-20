kubectl delete -f k8s/deployment.yaml
docker buildx build --platform linux/amd64,linux/arm64  \
	--target production \
	-t kasbench/globeco-allocation-service-server:latest \
	--push .
kubectl apply -f k8s/deployment.yaml
