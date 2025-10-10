#!/bin/bash

if [ -z "$1" ]; then
    echo "Usage: ./generate-password.sh <password>"
    exit 1
fi

docker run --rm python:3.11-slim sh -c "pip install -q bcrypt && python -c \"
import sys
import bcrypt
password = sys.argv[1].encode('utf-8')
hash = bcrypt.hashpw(password, bcrypt.gensalt())
print('Password hash:')
print(hash.decode('utf-8'))
print('\nAdd this to your docker-compose.yml:')
print(f'REALMRUNNER_PASSWORD_HASH: \\\"' + hash.decode('utf-8') + '\\\"')
\" '$1'"
