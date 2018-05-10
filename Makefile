


all:
	operator-sdk build quay.io/beekhof/fencing-operator

generated:
	operator-sdk generate k8s
