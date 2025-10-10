#!/usr/bin/env python3
import sys
import bcrypt

if len(sys.argv) < 2:
    print("Usage: python gen_password.py <password>")
    sys.exit(1)

password = sys.argv[1].encode('utf-8')
hash_bytes = bcrypt.hashpw(password, bcrypt.gensalt())
hash_str = hash_bytes.decode('utf-8')
escaped_hash = hash_str.replace('$', '$$')

print('Password hash (raw):')
print(hash_str)
print()
print('Add this to your docker-compose.yml (dollar signs escaped for YAML):')
print(f'REALMRUNNER_PASSWORD_HASH: "{escaped_hash}"')
