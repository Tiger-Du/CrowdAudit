FROM python:3.12-slim

COPY . .

WORKDIR /src

RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    software-properties-common \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install -r /requirements.txt

EXPOSE 80

HEALTHCHECK CMD curl --fail http://localhost:80/_stcore/health

ENTRYPOINT ["streamlit", "run", "./app.py", "--server.port=80", "--server.address=0.0.0.0"]