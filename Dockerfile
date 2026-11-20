# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o /out/katashiro ./cmd/katashiro \
 && go run ./tools/notices > /out/THIRD_PARTY_NOTICES

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/katashiro /usr/local/bin/katashiro
COPY LICENSE /usr/share/doc/katashiro/LICENSE
COPY --from=build /out/THIRD_PARTY_NOTICES /usr/share/doc/katashiro/THIRD_PARTY_NOTICES
ENV KATASHIRO_STATE_DIR=/home/nonroot/.local/state/katashiro
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/katashiro"]
CMD ["serve", "--port=8080", "--in-memory"]
