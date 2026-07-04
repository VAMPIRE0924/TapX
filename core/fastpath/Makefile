CC ?= gcc
CFLAGS ?= -std=c11 -O3 -Wall -Wextra -Werror -fPIC -pthread
AR ?= ar

BUILD_DIR := ../../build/fastpath
INCLUDES := -Iinclude
SRCS := src/tapx_fastpath.c
OBJS := $(patsubst src/%.c,$(BUILD_DIR)/%.o,$(SRCS))
LIB := $(BUILD_DIR)/libtapx_fastpath.a
TEST_BIN := $(BUILD_DIR)/tapx_fastpath_test

.PHONY: all clean test

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

clean:
	rm -rf $(BUILD_DIR)
