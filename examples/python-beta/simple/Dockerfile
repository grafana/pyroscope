FROM python:3.9

WORKDIR /usr/src/app

#RUN adduser --disabled-password --gecos --quiet pyroscope
#USER pyroscope

#COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope

USER root

COPY requirements.txt ./requirements.txt

# pip 22 is required
RUN python -m pip install --upgrade pip
# install rust toolchain
RUN curl https://sh.rustup.rs -sSf | sh -s -- --default-toolchain stable -y 
ENV PATH=$PATH:/root/.cargo/bin
# install libunwind, docker requires switching to root user
RUN apt-get update 
RUN apt-get install -y libunwind8-dev libssl-dev

RUN pip install -r requirements.txt

COPY main.py ./main.py

CMD ["python", "main.py"]
