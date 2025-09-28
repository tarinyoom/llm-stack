FROM python:3.12-alpine
RUN adduser -D -H -s /sbin/nologin app && mkdir -p /app && chown -R app:app /app
WORKDIR /app
COPY manage.py /app/manage.py
USER app
ENTRYPOINT ["python", "/app/manage.py"]

