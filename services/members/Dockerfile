# ---- build stage: full Go toolchain ----
FROM golang:1.23 AS build
WORKDIR /src

# Copy module files first so dependency download is cached separately
# from source changes (faster rebuilds). Zero deps today, but future-proof.
COPY go.mod ./
RUN go mod download

# Copy the rest of the repo and build just the members binary.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/members ./services/members

# ---- runtime stage: minimal, shell-less, nonroot ----
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/members /members
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/members"]
