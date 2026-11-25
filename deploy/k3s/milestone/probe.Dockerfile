FROM scratch
COPY milestone-probe /probe/milestone-probe
COPY migrations/ /probe/migrations/
ENTRYPOINT ["/probe/milestone-probe"]
