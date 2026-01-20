FROM python:3.13-slim

WORKDIR /app

# Install system dependencies for potential build requirements
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the application code
COPY app/ ./app/
COPY config.py .
COPY run.py .

EXPOSE 5000

ENV BASE_DOMAIN=short.example.com
ENV EXPIRY_HOURS=24
ENV SHORT_CODE_LENGTH=6
ENV DEFAULT_QR_COLOR="black"
ENV DEFAULT_QR_BACKGROUND="white"

# Use the entry point
CMD ["python", "run.py"]

