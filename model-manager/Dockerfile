FROM python:3.12-alpine
RUN adduser -D -H -s /sbin/nologin app && mkdir -p /app && chown -R app:app /app
WORKDIR /app
COPY model_manager.py /app/model_manager.py
USER app
ENTRYPOINT ["python", "/app/model_manager.py"]

