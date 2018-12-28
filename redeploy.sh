#!/bin/sh

apiserver-boot build container --image vmop
docker tag vmop derekbeard/vmoperator:vmop
docker push derekbeard/vmoperator:vmop
kubectl delete -f config/apiserver.yaml
kubectl apply -f config/apiserver.yaml --validate=false
