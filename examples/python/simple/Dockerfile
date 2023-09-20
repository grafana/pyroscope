FROM python:3.11

WORKDIR /usr/src/app

#RUN adduser --disabled-password --gecos --quiet pyroscope
#USER pyroscope
USER root

RUN python -m pip install --upgrade pip
COPY main.py ./main.py
COPY requirements.txt ./requirements.txt

RUN pip install -r requirements.txt

CMD ["python", "main.py"]
