FROM alpine:3.19

ARG CONTEXT_KIND

COPY . /context

RUN test -f /context/kept.txt

RUN if [ "$CONTEXT_KIND" = "root" ]; then \
        test ! -e /context/.git && \
        test ! -e /context/.github && \
        test ! -e /context/web && \
        test ! -e /context/bin && \
        test ! -e /context/coverage && \
        test ! -e /context/uploads && \
        test ! -e /context/.env && \
        test ! -e /context/debug.log && \
        test ! -e "/context/internal/source 2.go"; \
    elif [ "$CONTEXT_KIND" = "web" ]; then \
        test ! -e /context/node_modules && \
        test ! -e /context/.next && \
        test ! -e /context/coverage && \
        test ! -e /context/.env.local && \
        test ! -e /context/npm-debug.log && \
        test ! -e /context/tsconfig.tsbuildinfo && \
        test ! -e "/context/src/component 2.tsx" && \
        test ! -e "/context/public/icon 2.png"; \
    else \
        echo "unknown CONTEXT_KIND: $CONTEXT_KIND" >&2; \
        exit 1; \
    fi
