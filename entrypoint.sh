#!/bin/sh -l
cd /home/clr/mixer-tools

run_precheck() {
	make && sudo -E make install
	make lint && make check
}

run_battest() {
	make && sudo -E make install
	cd bat/tests/$TEST_DIR && make
}

if t=$(type -t "$INPUT_RUNFUNC"); then
	if [ "$t" = "function" ]; then
		$INPUT_RUNFUNC
	fi
fi
