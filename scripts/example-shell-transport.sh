#!/usr/bin/env bash

if [[ -z "${CARGOSHIP_FILE}" ]]; then
	echo "must set CARGOSHIP_FILE" before running 1>&2
	exit 2
fi

if [[ ! -e "${CARGOSHIP_FILE}" ]]; then
	echo "CARGOSHIP_FILE must be a file" 2>&2
	exit 3
fi

#rsync -va "${CARGOSHIP_FILE}" /tmp/
echo "Transporting off ${CARGOSHIP_FILE} to wherever it needs to be..."