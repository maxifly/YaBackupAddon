ARG BUILD_FROM
FROM $BUILD_FROM

ARG BUILD_ARCH

# Install requirements for add-on
RUN apk upgrade

RUN apk add --no-cache supervisor
RUN apk add go 

RUN apk upgrade

WORKDIR /app

# Copy data for add-on
COPY app/ .

WORKDIR /app/yabackup

COPY src/ .

RUN if [ "${BUILD_ARCH}" = "armhf" ]; then \
        export GOARCH=arm GOARM=6; \
    elif [ "${BUILD_ARCH}" = "armv7" ]; then \
        export GOARCH=arm GOARM=7; \
    elif [ "${BUILD_ARCH}" = "aarch64" ]; then \
        export GOARCH=arm64; \
    elif [ "${BUILD_ARCH}" = "i386" ]; then \
        export GOARCH=386; \
    elif [ "${BUILD_ARCH}" = "amd64" ]; then \
        export GOARCH=amd64; \
    else \
        exit 1; \
    fi \
    && go build -o ybg


WORKDIR /app

RUN chmod a+x ./run.sh

CMD ["/app/run.sh"]