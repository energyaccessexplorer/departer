default: clean build

-include ".env"

DEPARTER_CMD = departer \
	-role admin \
	-role leader \
	-role manager \
	-role director \
	-role root \
	-script ${DEPARTER_SCRIPT} \
	-tmpdir ${DEPARTER_TMPDIR} \
	-pubkey ${DEPARTER_PUBKEY}

build:
	go get
	go fmt
	go build

.export DEPARTER_CMD
.export DEPARTER_SOCKET
.export DEPARTER_WORKSPACE
.export DEPARTER_TMPDIR
.export DEPARTER_USER
.export OFFROAD_WORKSPACE
	@envsubst <departer.service-tmpl >departer.service
	@envsubst <departer.sh-tmpl >departer.sh

	@chmod +x departer.sh

clean:
	-rm -f departer departer.service departer.sh

install:
	git pull
	bmake build
	sudo install -o root -m 755 \
		departer \
		/usr/local/bin/

	sudo install -o root -m 755 \
		departer.sh \
		/usr/local/bin/

	sudo install -o root -g root -m 644 \
		departer.service \
		/etc/systemd/system/

deploy:
	rsync --recursive --verbose \
		--exclude="makefile" \
		--exclude="departer" \
		--exclude=".git*" \
		--exclude="go.mod" \
		./ eae-paver:~/departer

	ssh ${DEPARTER_SERVER} "sudo systemctl stop departer.service"
	ssh ${DEPARTER_SERVER} "cd ${DEPARTER_WORKSPACE}; bmake install;"
	ssh ${DEPARTER_SERVER} "sudo systemctl daemon-reload"
	ssh ${DEPARTER_SERVER} "sudo systemctl start departer.service"
