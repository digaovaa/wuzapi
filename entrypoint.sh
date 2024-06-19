#!/bin/sh

# Cria o arquivo .env e escreve as variáveis de ambiente nele
cat <<EOF > /app/.env
SECRET_KEY=${SECRET_KEY}
DB_HOST=${DB_HOST}
DB_USER=${DB_USER}
DB_PASSWORD=${DB_PASSWORD}
DB_NAME=${DB_NAME}
DB_PORT=${DB_PORT}
DB_DRIVER=${DB_DRIVER}
EOF

# Executa o comando original
exec "$@"
