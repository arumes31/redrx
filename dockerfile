# Dockerfile (updated)
FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .
COPY templates/ ./templates/
COPY static/ ./static/

EXPOSE 5000

ENV BASE_DOMAIN=r.reitetschlaeger.com
ENV EXPIRY_HOURS=24
ENV SHORT_CODE_LENGTH=6
ENV DEFAULT_QR_COLOR="black"
ENV DEFAULT_QR_BACKGROUND="white"

CMD ["python", "app.py"]