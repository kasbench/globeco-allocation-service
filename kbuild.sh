docker buildx build --platform linux/amd64,linux/arm64  \
	--target production \
	-t kasbench/globeco-allocation-service-server:latest \
	-t kasbench/globeco-allocation-service-server:1.0.3	\
	--push .
kubectl delete -f k8s/deployment.yaml
kubectl apply -f k8s/deployment.yaml
