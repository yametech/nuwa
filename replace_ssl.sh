export TLS_CRT=`cat ssl/tls.crt | base64`
export TLS_KEY=`cat ssl/tls.key | base64`
export CA_PEM=`cat ssl/ca.pem | base64`
sed -i "" "s/tls.crt\:.*$/tls.crt: ${TLS_CRT}/g" config/manager/secret.yaml
sed -i "" "s/tls.key\:.*$/tls.key: ${TLS_KEY}/g" config/manager/secret.yaml
sed -i "" "s/caBundle\:.*$/caBundle: ${TLS_CRT}/g" config/webhook/webhook.yaml
