###############################################################################
###                                CI	                                    ###
###############################################################################

include makefiles/ci.mk

include makefiles/test.mk

###############################################################################
###                                Build                                    ###
###############################################################################

include makefiles/build.mk

###############################################################################
###                            Docker Utils                                 ###
###############################################################################

kill:
	docker kill $$(docker ps -q)

clear:
	-docker compose down
	-docker image rm $$(docker image ls -q)
	-docker volume rm $$(docker volume ls -q)
	-docker rm -f $$(docker ps -a -q)
