FROM amazonlinux AS builder

WORKDIR /app

ARG GOOS
ENV GOOS=${GOOS:-linux}

ARG GOARCH
ENV GOARCH=${GOARCH:-arm64}
RUN dnf update -y && \
    dnf install -y wget tar && \
    dnf groupinstall -y "Development Tools" && \
    dnf install -y https://download.oracle.com/otn_software/linux/instantclient/instantclient-basic-linux-arm64.rpm && \
    dnf clean all
RUN wget -q https://go.dev/dl/go1.24.1.${GOOS}-${GOARCH}.tar.gz && \
        rm -rf /usr/local/go && \
        tar -C /usr/local -xzf go1.24.1.${GOOS}-${GOARCH}.tar.gz && \
        rm go1.24.1.${GOOS}-${GOARCH}.tar.gz
ENV PATH=$PATH:/usr/local/go/bin

COPY go.mod ./
COPY . ./
RUN go mod tidy && go mod vendor
RUN go build -o /app/collector cmd/collector/database-collector.go

FROM amazonlinux
WORKDIR /app
RUN dnf update -y && \
    dnf install -y https://download.oracle.com/otn_software/linux/instantclient/instantclient-basic-linux-arm64.rpm && \
    dnf clean all
COPY --from=builder /app/collector /app/database-collector
COPY --from=builder /app/exporters/oracle/custom-metrics.toml /app/oracle-custom-metrics.toml
CMD ["/app/database-collector"]
