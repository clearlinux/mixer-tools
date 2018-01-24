SUBMAKEFILES := $(shell find bat/tests/ -name Makefile)
DIRS2RUNMAKECHECK := $(addprefix checkdir-,${SUBMAKEFILES})
DIRS2RUNMAKECLEAN := $(addprefix clean-,${SUBMAKEFILES})

batcheck: ${DIRS2RUNMAKECHECK}

${DIRS2RUNMAKECHECK}: checkdir-%:
	$(MAKE) -C $(dir $(subst checkdir-,,$@)) check

batclean: $(DIRS2RUNMAKECLEAN)

${DIRS2RUNMAKECLEAN}: clean-%:
	$(MAKE) -C $(dir $(subst clean-,,$@)) clean

.PHONY: batcheck
.PHONY: batclean
.PHONY: ${DIRS2RUNMAKECHECK}
.PHONY: ${DIRS2RUNMAKECLEAN}
