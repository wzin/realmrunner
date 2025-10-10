#!/usr/bin/env python3
import sys
import bcrypt

if len(sys.argv) < 2:
    print("Usage: python gen_password.py <password>", file=sys.stderr)
    sys.exit(1)

password = sys.argv[1].encode('utf-8')
hash_bytes = bcrypt.hashpw(password, bcrypt.gensalt())
hash_str = hash_bytes.decode('utf-8')
escaped_hash = hash_str.replace('$', '$$')

# Output ONLY the escaped hash - ready to paste into docker-compose.yml
print(escaped_hash)
