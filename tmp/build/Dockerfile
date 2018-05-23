FROM alpine:3.6

ADD tmp/_output/bin/fencing-operator /usr/local/bin/fencing-operator

RUN adduser -D fencing-operator
USER fencing-operator
CMD /usr/local/bin/fencing-operator