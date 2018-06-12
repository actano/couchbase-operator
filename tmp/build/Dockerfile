FROM alpine:3.6

RUN adduser -D couchbase-operator
USER couchbase-operator

ADD tmp/_output/bin/couchbase-operator /usr/local/bin/couchbase-operator
