[![build status](http://git.tmaws.io/kubernetes/alb-ingress/badges/master/build.svg)](http://git.tmaws.io/kubernetes/alb-ingress/commits/master) [![coverage report](http://git.tmaws.io/kubernetes/alb-ingress/badges/master/coverage.svg)](http://git.tmaws.io/kubernetes/alb-ingress/commits/master)

The ingress code is developed on an ubuntu box and so you need to set your mac up to have some directories/files

```
$ glide install -v
$ go build
```

```
bigkraigs-macbook-pro:alb-ingress kamador$ cat /etc/ssl/certs/ssl-cert-snakeoil.pem
-----BEGIN CERTIFICATE-----
MIICvjCCAaagAwIBAgIJAOh3ARkR6gsfMA0GCSqGSIb3DQEBCwUAMBcxFTATBgNV
BAMMDGJiMGY0MmMxNTZkNjAeFw0xNzAyMDQwMzQxNTVaFw0yNzAyMDIwMzQxNTVa
MBcxFTATBgNVBAMMDGJiMGY0MmMxNTZkNjCCASIwDQYJKoZIhvcNAQEBBQADggEP
ADCCAQoCggEBAOvJGCueca2mWwZ9DgN6oXjqBaPromLYgiS/TGK98GqQMtM+WXUc
rBEiEA2b2Mt4UMPKh+s71gZO3SjLWOjF72v5UKzPWGVzi6scPMxHSHn2QYIrYcKq
9g2m6PFr+0C3G1nLGiwWBtoHz5OF9VFGcfnHDjPwkLxBqi5jihbVo0YOmJkbsXNA
+7lrjdNyBoHE3ESFlWxX9Aq9fQOu5/kWwCtrFrwQFbN7+owdQ7MvJbxVySwbn9TF
GK+EIoeEay9x2BRbtUIWvM3By2+0urrMlKhOSaXSwgXMVeZwHJMMANIMxgJuUIm1
ndZabTWn4jrWhDpQKxFgs4JMRK0pOi57wPUCAwEAAaMNMAswCQYDVR0TBAIwADAN
BgkqhkiG9w0BAQsFAAOCAQEAAPwADGMvofWdW5N1WsIr92PxJ+8GMsulLhMTkQWp
6k9zqX6kC5TBa4vDOebq1lQjLLGd3Cwejgo3rnnNJi01IIZc8/vby+944YLGpXEE
h1sEU+rBEbCjTlMaMnRmg0+EyJMn0kE0XVScBUYFB1yyg62mOk2YtWbD9eleq1CE
gt/1wKokuMXMMpCaWz06KRpoNPOW5BB9czva8qXuPVrEvF1KD7FllgRU5/Es7ahf
gtmUK3SQcVQH08K5PfiWTTboQ8ImKUSsynNaK3/CiiMSZkxnUYWEexqOx0p3XaWR
aocjs6hBLwO3OHaOBo4RX7Gy4HnWJWhUxsHuWWm7YqczVg==
-----END CERTIFICATE-----

bigkraigs-macbook-pro:alb-ingress kamador$ cat /etc/ssl/private/ssl-cert-snakeoil.key
-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDryRgrnnGtplsG
fQ4DeqF46gWj66Ji2IIkv0xivfBqkDLTPll1HKwRIhANm9jLeFDDyofrO9YGTt0o
y1joxe9r+VCsz1hlc4urHDzMR0h59kGCK2HCqvYNpujxa/tAtxtZyxosFgbaB8+T
hfVRRnH5xw4z8JC8QaouY4oW1aNGDpiZG7FzQPu5a43TcgaBxNxEhZVsV/QKvX0D
ruf5FsAraxa8EBWze/qMHUOzLyW8VcksG5/UxRivhCKHhGsvcdgUW7VCFrzNwctv
tLq6zJSoTkml0sIFzFXmcByTDADSDMYCblCJtZ3WWm01p+I61oQ6UCsRYLOCTESt
KToue8D1AgMBAAECggEAVEYL3YtEFkzfO/J2j8fE7vK2EWCnKa041umI48H/rBbe
E6K1VqZo2bbTBgot8ouOUmyRKAK6/IYzheEoZgpZCL6TlzCE573krcPp9xmDThQ2
VdAroOh6CWce2ys9ImRP4kg1koxM5qDkEFZQ2DvVgPEkomvZT3Ao42uwb3jsYp0l
NUfc+8kMhsqIUPbOTNvQGbGjaYHBnPIArBgmcfvRtqIdji0moiNpvg6t63zC2RlM
RSzXLA/gTF41/2u+ONvkt5JLFjw+Rs7hgNGxgK+xna3pQHiyc586cu7RANo+zQ9B
GVM3mFq4a+dbziWe/KQyntMUFgu+rcJvcN6k215gAQKBgQD91iW+m6CPA9Qyu+zw
CHUEMkJRL9Oe3MLYWqdoy5jY3ATyWjZhnIoHBlbwjK5de3eX6thh7zNwkGlhYosm
J2NTtaShTq5AIS8rEnUOJN4WRwhO2mtjMglSeNs3LblIT0pVf8XGQE/sXx8ID6Vc
WNRH3Kt5l+v20cCApZfnX4UaUQKBgQDty4+g92LIKiXhFiFsd3PmLNKvp9OZY/jr
rPRorzc35TcO3u+CQuahUhkUQ2nksSwfb4B74D78clsUDjjyGI7kro0U6CMEVbJc
WH4TNFhCx7+b3ITj8D8DOsUdh9K5eul0Q3xKKQWvS4TA5D7h1hVN0bC6Hi3dooty
VQ2WuIuvZQKBgQCFlv2YWhlfCxnTdZnWHe1Pvw+t4KjUE8UrzlIK0hPoFas4zQeP
ya3O0qRQxwlBQ6iGOF+W8ye0VxxO12j6NIKO3Kr/BgSo1Y4YcgdO4VJMkSerMEKS
GxRS5i4g1RyNFMW/R5aTpucpNEqFmI5jkpBuHZHVVYInDO7uBbhzWY8YcQKBgQDJ
yH6snMAaEonqIpliLUsP+uEdZVBNTWQguLb9ThGRQNQjrlGXO6XxJnVZmIr3INDM
LBXfCD7qgS+AKUFxTh2TN/tHzmRIfV/ItN7m9PggUtfpLosl0OvnlatGj8bk1cPc
gJerZnwIcBDKjeQ+Ryf0zQcmKA3LkO67qijJyPffcQKBgAR9g0QXjuiA5Z8yf3d0
3n4udnlTpkgWupYLJV0ZQLTD7Vwx5sD47AyZovXYXmabQY7ijJyowENZ6PfHrOLh
XUZTSZjevqXlD5XcfpAys5BQIxxGKFE2d8yNRrftBa/6IxqAJK7eLC+KkTQecSVm
HgP2KDRt4IApeQevqZ6u/rYA
-----END PRIVATE KEY-----

bigkraigs-macbook-pro:alb-ingress kamador$ ls -la /ingress-controller/ssl/
total 160
drwxr-xr-x  22 kamador  wheel  748 Feb  3 20:09 .
drwxr-xr-x   3 kamador  wheel  102 Feb  3 18:52 ..

MacBook-Pro:alb-ingress kamador$ POD_NAMESPACE=default NOOP=true AWS_REGION=us-east-1 AWS_PROFILE=tm-nonprod-Ops-Techops CLUSTER_NAME=dev ./alb-ingress --apiserver-host http://127.0.0.1:8001 --default-backend-service kube-system/default-http-backend 
I0209 16:44:10.612695   12299 launch.go:92] &{ALB Controller 0.0.1 git-00000000 git://git.tmaws.io/kubernetes/alb-ingress-controller}
I0209 16:44:10.612903   12299 launch.go:221] Creating API server client for http://127.0.0.1:8001
I0209 16:44:10.693417   12299 launch.go:111] validated kube-system/default-http-backend as the default backend
I0209 16:44:10.773888   12299 controller.go:1014] starting Ingress controller

```
