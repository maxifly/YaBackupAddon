ARG BUILD_FROM
FROM $BUILD_FROM

# Install requirements for add-on
RUN apk upgrade

#RUN \
#  apk add --no-cache \
#    git \
#    supervisor \
#    python3 \
#    py3-pip

RUN \
  apk add --no-cache \
    git \
    supervisor \
    python3 \
    py3-pip

RUN apk upgrade

#RUN \
#  apk add --no-cache \
#    python3 \
#    py3-pip

#RUN apt-get -y install python3-pip
RUN pip3 install flask 
RUN pip3 install yadisk==1.3.0 
RUN pip3 install python-crontab
#RUN pip3 install supervisor-stdout
#RUN pip3 install git+https://github.com/coderanger/supervisor-stdout
RUN pip3 install supervisor-console

# Python 3 HTTP Server serves the current working dir
# So let's set it to our add-on persistent data directory.


WORKDIR /os
COPY os/ .

WORKDIR /app


# Copy data for add-on
COPY src/ .

RUN chmod a+x ./run.sh
RUN chmod a+x ./run1.sh
#RUN crontab /os/cron.txt

#CMD /usr/sbin/crond -d 0 && /app/run.sh
CMD ["/usr/bin/supervisord","-c","/os/supervisord.conf"]
#CMD ["/app/run.sh"]