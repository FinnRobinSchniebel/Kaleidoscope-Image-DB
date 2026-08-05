#!/bin/sh
set -e

gen() {
  head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'
}

[ -f /secrets/mongo_user ]      || echo "admin" > /secrets/mongo_user
[ -f /secrets/mongo_password ]  || gen > /secrets/mongo_password
[ -f /secrets/jwt_secret ]      || gen > /secrets/jwt_secret
[ -f /secrets/password_pepper ] || gen > /secrets/password_pepper

# Backend runs as a non-root user (see Dockerfile.backend), so make sure the
# files it reads from are world-readable regardless of this image's umask.
chmod 644 /secrets/mongo_user /secrets/mongo_password /secrets/jwt_secret /secrets/password_pepper

echo "secrets ready"
