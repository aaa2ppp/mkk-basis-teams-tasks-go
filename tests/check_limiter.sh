#!/bin/sh

for i in {1..40}; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/sign/in
  sleep 0.01
done
