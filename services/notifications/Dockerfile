FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/notifications ./services/notifications

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/notifications /notifications
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/notifications"]
