// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

#define VK_USE_PLATFORM_ANDROID_KHR 1

#include <stddef.h>
#include <vulkan/vulkan.h>

_Static_assert(sizeof(void *) == 8, "Android preview requires LP64");
_Static_assert(sizeof(VkInstance) == 8, "unexpected VkInstance size");
_Static_assert(sizeof(VkSurfaceKHR) == 8, "unexpected VkSurfaceKHR size");
_Static_assert(sizeof(VkAndroidSurfaceCreateInfoKHR) == 32,
               "unexpected VkAndroidSurfaceCreateInfoKHR size");
_Static_assert(offsetof(VkAndroidSurfaceCreateInfoKHR, sType) == 0,
               "unexpected sType offset");
_Static_assert(offsetof(VkAndroidSurfaceCreateInfoKHR, pNext) == 8,
               "unexpected pNext offset");
_Static_assert(offsetof(VkAndroidSurfaceCreateInfoKHR, flags) == 16,
               "unexpected flags offset");
_Static_assert(offsetof(VkAndroidSurfaceCreateInfoKHR, window) == 24,
               "unexpected window offset");
