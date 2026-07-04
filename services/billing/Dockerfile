FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/billing ./services/billing

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/billing /billing
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/billing"]
