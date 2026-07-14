# Sonar Scanner for data-agent (Go project)
FROM sonarsource/sonar-scanner-cli:latest

USER root
RUN python3 -m pip install --quiet --no-cache-dir requests && python3 -m pip cache purge
USER scanner-cli

COPY --chown=scanner-cli:scanner-cli entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

COPY --chown=scanner-cli:scanner-cli parse_report.py /parse_report.py
RUN chmod +r /parse_report.py

ENTRYPOINT ["/entrypoint.sh"]
