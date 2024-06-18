FROM golang:1.22-alpine
RUN mkdir /app
COPY . /app
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
RUN go build -o server .
VOLUME [ "/app/dbdata", "/app/files" ]
ENTRYPOINT [ "/app/server" ]
CMD [ "-logtype", "console" ]
