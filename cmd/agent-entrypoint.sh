#!/bin/bash

extra_args=""
env | grep SECRETPATH_ | sed s/SECRETPATH_// > /tmp/secrets

if [ x$SECRET_FORMAT = x args ]; then
    while IFS= read -r line; do
	field=$(echo $line | awk -F= '{print $1}')
	secretpath=$(echo $line | awk -F= '{print $1}')
	extra_args="--$field $(cat $secretpath)"
    done < /tmp/secrets
else
    while IFS= read -r line; do
	export "$line"
    done < /tmp/secrets
fi

$* $extra_args
