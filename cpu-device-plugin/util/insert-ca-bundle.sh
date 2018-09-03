#!/bin/bash

CA_BUNDLE=$(kubectl get configmap -n kube-system extension-apiserver-authentication -o=jsonpath='{.data.client-ca-file}' | base64 | tr -d '\n')

#echo $CA_BUNDLE
cat $1 | sed -e "s/BASE64_ENCODED_CERT/${CA_BUNDLE}/"
