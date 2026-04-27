.PHONY: schedule-install schedule-uninstall schedule-test build-shared

REPO_DIR := $(shell pwd)
LAUNCH_AGENT_LABEL := com.mikelady.csms-weekly-review
LAUNCH_AGENT_DIR := $(HOME)/Library/LaunchAgents
LAUNCH_AGENT_PLIST := $(LAUNCH_AGENT_DIR)/$(LAUNCH_AGENT_LABEL).plist

# Build all _shared/ Go binaries. Idempotent.
build-shared:
	@for d in _shared/buffer-post-prep _shared/buffer-queue-check _shared/voice-corpus; do \
		echo "--- $$d"; \
		(cd $$d && go build .) || exit 1; \
	done

# Install the Sunday 09:30am weekly-review LaunchAgent. Materializes the
# template at scripts/com.mikelady.csms-weekly-review.plist with absolute
# paths into ~/Library/LaunchAgents/ and loads it with launchctl.
#
# Sunday 09:30 is staggered 30 minutes after the yt-analytics review at
# 09:00 so the two crons don't compete for Claude API quota or browser focus.
schedule-install:
	@mkdir -p $(LAUNCH_AGENT_DIR)
	@mkdir -p $(HOME)/Library/Logs/csms-weekly-review
	@sed -e 's|__REPO_DIR__|$(REPO_DIR)|g' -e 's|__HOME__|$(HOME)|g' \
		scripts/$(LAUNCH_AGENT_LABEL).plist > $(LAUNCH_AGENT_PLIST)
	@launchctl unload $(LAUNCH_AGENT_PLIST) 2>/dev/null || true
	@launchctl load $(LAUNCH_AGENT_PLIST)
	@echo "Installed $(LAUNCH_AGENT_LABEL) — fires Sundays at 9:30 AM local."
	@echo "Logs: ~/Library/Logs/csms-weekly-review/"
	@echo "Verify: launchctl list | grep csms-weekly-review"

# Remove the LaunchAgent. Idempotent.
schedule-uninstall:
	@launchctl unload $(LAUNCH_AGENT_PLIST) 2>/dev/null || true
	@rm -f $(LAUNCH_AGENT_PLIST)
	@echo "Uninstalled $(LAUNCH_AGENT_LABEL)."

# Run the weekly-review script immediately (does NOT wait for Sunday).
# Useful for testing the install end-to-end.
schedule-test:
	bash scripts/weekly-review.sh
