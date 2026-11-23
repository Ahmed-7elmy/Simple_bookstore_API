FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bookstore-api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bookstore-api /bookstore-api
EXPOSE 8080
ENTRYPOINT ["/bookstore-api"]
