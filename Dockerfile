FROM reverbrain/trusty-dev

#RUN	echo "deb http://repo.reverbrain.com/trusty/ current/amd64/" > /etc/apt/sources.list.d/reverbrain.list && \
#	echo "deb http://repo.reverbrain.com/trusty/ current/all/" >> /etc/apt/sources.list.d/reverbrain.list && \
#	apt-get install -y curl && \
#	curl http://repo.reverbrain.com/REVERBRAIN.GPG | apt-key add - && \
#	apt-get update && \
#	apt-get upgrade -y && \
#	apt-get install -y curl git elliptics elliptics-dev elliptics-client ebucket ebucket-dev make && \
#	cp -f /usr/share/zoneinfo/posix/W-SU /etc/localtime && \
#	echo Europe/Moscow > /etc/timezeone

RUN	VERSION=go1.6.3 && \
	curl -O https://storage.googleapis.com/golang/$VERSION.linux-amd64.tar.gz && \
	rm -rf /usr/local/go && \
	tar -C /usr/local -xf $VERSION.linux-amd64.tar.gz && \
	rm -f $VERSION.linux-amd64.tar.gz

RUN	git config --global user.email "zbr@ioremap.net" && \
	git config --global user.name "Evgeniy Polyakov" && \
	export PATH=$PATH:/usr/local/go/bin && \
	export GOPATH=/root/awork/go && \
	mkdir -p ${GOPATH} && \
	rm -rf ${GOPATH}/pkg/* && \
	rm -rf ${GOPATH}/src/github.com/bioothod/ljsearch_server && \
	go get github.com/bioothod/ljsearch_server && \
	cd ${GOPATH}/src/github.com/bioothod/ljsearch_server && \
	git branch -v && \
	go install && \
	echo "Livejournal search server has been updated and installed" ;\
    	rm -rf /var/lib/apt/lists/*

EXPOSE 8080 80 8111
