cat >  ca-config.json <<EOF
{
"signing": {
"default": {
  "expiry": "8760h"
},
"profiles": {
  "kubernetes-Soulmate": {
    "usages": [
        "signing",
        "key encipherment",
        "server auth",
        "client auth"
    ],
    "expiry": "8760h"
  }
}
}
}
EOF

cat >  ca-csr.json <<EOF
{
"CN": "kubernetes-Soulmate",
"key": {
"algo": "rsa",
"size": 2048
},
"names": [
{
  "C": "${COUNTRY}",
  "ST": "${CITY}",
  "L": "${CITY}",
  "O": "k8s",
  "OU": "System"
}
]
}
EOF


if [ ! $NUWA_DEV_IP ]; then
  HOSTS='[
        "nuwa-controller-manager-service.nuwa-system.svc",
        "nuwa-controller-manager-service.svc.cluster.local",
        "nuwa-sidecar-injector.svc.cluster.local",
        "nuwa-controller-manager-metrics-service.svc.cluster.local"
        ]'
else
  # shellcheck disable=SC2016
  HOSTS='[ "${NUWA_DEV_IP}" ]'
fi

if [ ! $COUNTRY ]; then
COUNTRY=CN
fi
if [ ! $CITY ]; then
CITY=GuangZhou
fi

cat > tls-csr.json <<EOF
{
  "CN": "nuwa",
	"hosts": $HOSTS,
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "${COUNTRY}",
      "ST": "${CITY}",
      "L": "${CITY}",
      "O": "k8s",
      "OU": "System"
    }
  ]
}
EOF

cfssl gencert -initca ca-csr.json | cfssljson -bare ca
cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=kubernetes-Soulmate tls-csr.json | cfssljson -bare tls

openssl x509  -noout -text -in tls.pem
openssl x509  -noout -text -in tls-key.pem
openssl x509  -noout -text -in ca.pem

mv tls-key.pem tls.key
mv tls.pem tls.crt

echo "tls.crt\r\n----------------------------------------------------------------"
cat tls.crt | base64
echo "----------------------------------------------------------------\r\n"
echo "tls.key\r\n----------------------------------------------------------------"
cat tls.key | base64
echo "----------------------------------------------------------------\r\n"
