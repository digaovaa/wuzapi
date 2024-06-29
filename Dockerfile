FROM golang:1.22-alpine
RUN mkdir /app
WORKDIR /app
COPY . /app
RUN apk add --no-cache gcc musl-dev
RUN go build -o server .
VOLUME [ "/app/dbdata", "/app/files" ]
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh
ENTRYPOINT [ "entrypoint.sh","/app/server" ]
CMD [ "-logtype", "console" ]
ENV DOCKERIZE_VERSION v0.7.0
# RUN apk add --no-cache dumb-init \
#     && ARCH=$(apk --print-arch) \
#     && if [ "$ARCH" = "x86_64" ]; then \
#            wget https://github.com/jwilder/dockerize/releases/download/$DOCKERIZE_VERSION/dockerize-linux-amd64-$DOCKERIZE_VERSION.tar.gz; \
#        elif [ "$ARCH" = "aarch64" ]; then \
#            wget https://github.com/jwilder/dockerize/releases/download/$DOCKERIZE_VERSION/dockerize-linux-arm64-$DOCKERIZE_VERSION.tar.gz; \
#        else \
#            echo "Unsupported architecture: $ARCH"; exit 1; \
#        fi \
#     && tar -C /usr/local/bin -xzvf dockerize-linux-*-$(echo $DOCKERIZE_VERSION).tar.gz \
#     && rm dockerize-linux-*-$(echo $DOCKERIZE_VERSION).tar.gz
