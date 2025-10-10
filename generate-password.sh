#!/bin/bash

if [ -z "$1" ]; then
    echo "Usage: ./generate-password.sh <password>"
    exit 1
fi

docker run --rm -v "$(pwd)/gen_password.py:/gen_password.py" python:3.11-slim sh -c \
    "pip install -q bcrypt && python /gen_password.py '$1'"
