.PHONY: backend-test backend-run backend-migrate-up backend-migrate-status

backend-test:
	$(MAKE) -C backend test

backend-run:
	$(MAKE) -C backend run

backend-migrate-up:
	$(MAKE) -C backend migrate-up

backend-migrate-status:
	$(MAKE) -C backend migrate-status
