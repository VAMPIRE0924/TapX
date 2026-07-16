CC ?= gcc
CFLAGS ?= -std=c11 -O3 -Wall -Wextra -Werror -fPIC -pthread
AR ?= ar

BUILD_DIR := ../../build/fastpath
INCLUDES := -Iinclude
SRCS := src/tapx_fastpath.c
OBJS := $(patsubst src/%.c,$(BUILD_DIR)/%.o,$(SRCS))
LIB := $(BUILD_DIR)/libtapx_fastpath.a
TEST_BIN := $(BUILD_DIR)/tapx_fastpath_test
SANITIZER_CFLAGS := -std=c11 -O1 -g -Wall -Wextra -Werror -fPIC -pthread -fsanitize=address,undefined -fno-omit-frame-pointer

.PHONY: all clean test sanitize analyze

all: $(LIB)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(BUILD_DIR)/%.o: src/%.c include/tapx_fastpath.h | $(BUILD_DIR)
	$(CC) $(CFLAGS) $(INCLUDES) -c $< -o $@

$(LIB): $(OBJS)
	$(AR) rcs $@ $^

test: $(LIB) tests/tapx_fastpath_test.c
	$(CC) $(CFLAGS) $(INCLUDES) tests/tapx_fastpath_test.c $(LIB) -o $(TEST_BIN)
	$(TEST_BIN)

sanitize:
	$(MAKE) clean
	$(MAKE) test CC=clang CFLAGS="$(SANITIZER_CFLAGS)"

analyze:
	clang --analyze -std=c11 -Wall -Wextra -Werror -pthread $(INCLUDES) src/tapx_fastpath.c

clean:
	rm -rf $(BUILD_DIR)
