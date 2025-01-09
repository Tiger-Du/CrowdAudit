# syntax=docker/dockerfile:1

FROM python:3.12-slim

RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    software-properties-common \
    && rm -rf /var/lib/apt/lists/*

RUN --mount=type=bind,source=requirements.txt,destination=/tmp/requirements.txt \
    pip install -r /tmp/requirements.txt

COPY ./src .

EXPOSE 8501

HEALTHCHECK CMD curl --fail http://localhost:8501/_stcore/health

ENTRYPOINT ["streamlit", "run", "./app.py", "--server.port=8501", "--server.address=0.0.0.0"]
