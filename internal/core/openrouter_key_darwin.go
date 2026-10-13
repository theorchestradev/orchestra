//go:build darwin && cgo

package core

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static OSStatus orchestraKeychain(const char *service, int operation, const char *secret, char **output, int *length) {
    CFStringRef name = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
    CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
    CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
    CFDictionarySetValue(query, kSecAttrService, name);
    CFDictionarySetValue(query, kSecAttrAccount, CFSTR("management-key"));
    OSStatus status;
    if (operation == 0) {
        CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
        CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
        CFDataRef data = NULL;
        status = SecItemCopyMatching(query, (CFTypeRef *)&data);
        if (status == errSecSuccess) {
            *length = (int)CFDataGetLength(data);
            *output = malloc(*length);
            if (*output) memcpy(*output, CFDataGetBytePtr(data), *length);
            else status = errSecAllocate;
            CFRelease(data);
        }
    } else if (operation == 1) {
        CFDataRef data = CFDataCreate(NULL, (const UInt8 *)secret, strlen(secret));
        const void *keys[] = {kSecValueData};
        const void *values[] = {data};
        CFDictionaryRef update = CFDictionaryCreate(NULL, keys, values, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
        status = SecItemUpdate(query, update);
        if (status == errSecItemNotFound) {
            CFDictionarySetValue(query, kSecValueData, data);
            CFDictionarySetValue(query, kSecAttrLabel, CFSTR("Orchestra OpenRouter management key"));
            status = SecItemAdd(query, NULL);
        }
        CFRelease(update);
        CFRelease(data);
    } else {
        status = SecItemDelete(query);
    }
    CFRelease(query);
    CFRelease(name);
    return status;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func OpenRouterKey(service string) (string, error) { return openRouterKeychain(service, 0, "") }
func SaveOpenRouterKey(service, key string) error {
	_, err := openRouterKeychain(service, 1, key)
	return err
}
func RemoveOpenRouterKey(service string) error {
	_, err := openRouterKeychain(service, 2, "")
	return err
}

func openRouterKeychain(service string, operation int, secret string) (string, error) {
	name, value := C.CString(service), C.CString(secret)
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(value))
	var output *C.char
	var length C.int
	status := C.orchestraKeychain(name, C.int(operation), value, &output, &length)
	if output != nil {
		defer C.free(unsafe.Pointer(output))
	}
	if status == C.errSecItemNotFound && operation != 1 {
		return "", nil
	}
	if status != C.errSecSuccess {
		return "", errors.New("Could not access the OpenRouter key in macOS Keychain. Allow Orchestra access and try again.")
	}
	if output != nil {
		return C.GoStringN(output, length), nil
	}
	return "", nil
}
