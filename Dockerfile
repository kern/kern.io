FROM ubuntu:12.04
EXPOSE 80

RUN apt-get update && apt-get upgrade -y
RUN apt-get install -y git nodejs npm

RUN rm -rf /tmp/kern.io
RUN git clone git@github.com:kern/kern.io.git /tmp/kern.io

WORKDIR /tmp/kern.io
RUN npm install
CMD npm start
