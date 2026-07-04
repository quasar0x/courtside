FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/clubs ./services/clubs

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/clubs /clubs
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/clubs"]
