FROM golang:1.22-alpine
RUN mkdir /app
COPY . /app
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
RUN go build -o server .
VOLUME [ "/app/dbdata", "/app/files" ]
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh
ENTRYPOINT [ "entrypoint.sh","/app/server" ]
CMD [ "-logtype", "console" ]
