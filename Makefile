# Proxy all make commands to go-task
.DEFAULT_GOAL := _default

_default:
	@task

%:
	@task $@
