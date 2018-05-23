NS=test
KUBECTL=kubectl -n $(NS)

all: build

build:
	operator-sdk build quay.io/beekhof/fencing-operator

push: build
	docker push quay.io/beekhof/fencing-operator

generated:
	operator-sdk generate k8s

ns:
	-kubectl create ns $(NS)
	-example/rbac/create_role.sh  --namespace $(NS) --role-name $(NS)-operator --role-binding-name $(NS)-operator


run:	clean ns 
	$(KUBECTL) create -f example/operator.yaml
	while [ "x$$($(KUBECTL) get po | grep fencing-operator.*Running)" = x ]; do sleep 5; /bin/echo -n .; done
	echo "."
	$(KUBECTL) logs -f fencing-operator

logs:
	$(KUBECTL) logs -f fencing-operator

clean:
	-$(KUBECTL) delete -f example/operator.yaml
	while [ "x$$($(KUBECTL) get po 2>/dev/null)" != "x" ]; do sleep 5; /bin/echo -n .; done
	echo "."
	-kubectl delete ns/$(NS) clusterrole/$(NS)-operator clusterrolebinding/$(NS)-operator
	while [ "x$$(kubectl get ns $(NS) 2>/dev/null)" != "x" ]; do sleep 5; /bin/echo -n .; done
	echo "."

