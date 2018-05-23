NS=test
KUBECTL=kubectl -n $(NS)
export GREP=grep --color=never
OPERATOR=example/deployment-operator.yaml
MASTER=$$( for POD in $$( $(KUBECTL) get po | $(GREP) fencing-operator.*Running | awk '{print $$1}' ); do $(KUBECTL) logs po/$$POD ; done | sort | /bin/grep "became leader" | sed s/.*LeaderElection\'// | awk '{print $$1}')

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


run:	clean clean-ns ns 
	$(KUBECTL) create -f $(OPERATOR)
	echo -n "Waiting..."
	while [ "x$$( $(KUBECTL) get po | grep fencing-operator-.*Running)" = x ]; do sleep 5; /bin/echo -n .; done
	echo " done"

logs:
	$(KUBECTL) describe pods | grep -e ^Name: -e ^Node: | sed 's/Node:/     /'
	echo $(KUBECTL) logs -f $(MASTER) 
	$(KUBECTL) logs -f $(MASTER) 

clean:
	-$(KUBECTL) delete -f $(OPERATOR)
	echo -n "Waiting..."
	while [ "x$$($(KUBECTL) get po 2>/dev/null)" != "x" ]; do sleep 5; /bin/echo -n .; done
	echo " done"

clean-ns:
	-kubectl delete ns/$(NS) clusterrole/$(NS)-operator clusterrolebinding/$(NS)-operator
	echo -n "Waiting..."
	while [ "x$$(kubectl get ns $(NS) 2>/dev/null)" != "x" ]; do sleep 5; /bin/echo -n .; done
	echo " done"

