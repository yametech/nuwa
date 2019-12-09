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
  "C": "CN",
  "ST": "GuangZhou",
  "L": "GuangZhou",
  "O": "k8s",
  "OU": "System"
}
]
}
EOF

cat > tls-csr.json <<EOF
{
  "CN": "nuwa",
	"hosts": [
    "10.1.140.202",
    "10.1.140.83",
    "10.1.180.126"
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "CN",
      "ST": "GuangZhou",
      "L": "GuangZhou",
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
cat tls.crt | base64