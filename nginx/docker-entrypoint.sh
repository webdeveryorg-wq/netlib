#!/bin/sh
set -e

# Домен из ENV
NGINX_DOMAIN="${NGINX_DOMAIN}"
CERT_PATH="/etc/letsencrypt/live/$NGINX_DOMAIN"
TEMP_CERT_PATH="/etc/nginx/ssl/temp"

echo "Используется домен: $NGINX_DOMAIN"

# Создаём директорию для ACME challenge
mkdir -p /var/www/certbot/.well-known/acme-challenge
chmod -R 755 /var/www/certbot

# Создаём директорию для временного сертификата
mkdir -p "$TEMP_CERT_PATH"

# Проверяем наличие настоящего сертификата от Let's Encrypt
if [ -f "$CERT_PATH/fullchain.pem" ] && [ -f "$CERT_PATH/privkey.pem" ]; then
    echo "Найден сертификат Let's Encrypt"
    SSL_CERT="$CERT_PATH/fullchain.pem"
    SSL_KEY="$CERT_PATH/privkey.pem"
else
    echo "Сертификат Let's Encrypt не найден. Создаём временный самоподписанный..."
    
    # Устанавливаем openssl если его нет
    apk add --no-cache openssl
    
    # Генерируем временный самоподписанный сертификат в отдельной директории
    openssl req -x509 -nodes -days 1 -newkey rsa:2048 \
        -keyout "$TEMP_CERT_PATH/privkey.pem" \
        -out "$TEMP_CERT_PATH/fullchain.pem" \
        -subj "/CN=$NGINX_DOMAIN" 2>/dev/null
    
    SSL_CERT="$TEMP_CERT_PATH/fullchain.pem"
    SSL_KEY="$TEMP_CERT_PATH/privkey.pem"
    
    echo "Временный сертификат создан. Certbot заменит его на настоящий."
fi

# Генерируем nginx.conf из шаблона с подстановкой переменных
export SSL_CERT SSL_KEY
envsubst '${NGINX_DOMAIN} ${SSL_CERT} ${SSL_KEY}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf

# Запускаем цикл проверки сертификата и перезагрузки nginx
(
    while :; do
        sleep 5
        # Проверяем появился ли настоящий сертификат
        if [ -f "$CERT_PATH/fullchain.pem" ] && [ -f "$CERT_PATH/privkey.pem" ]; then
            # Обновляем конфиг с настоящим сертификатом
            SSL_CERT="$CERT_PATH/fullchain.pem"
            SSL_KEY="$CERT_PATH/privkey.pem"
            export SSL_CERT SSL_KEY
            envsubst '${NGINX_DOMAIN} ${SSL_CERT} ${SSL_KEY}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf
            echo "Перезагрузка nginx с сертификатом Let's Encrypt..."
            nginx -s reload
            # После успешной перезагрузки ждём дольше
            sleep 6h
        fi
    done
) &

# Запуск nginx
echo "Запуск nginx..."
exec nginx -g "daemon off;"
