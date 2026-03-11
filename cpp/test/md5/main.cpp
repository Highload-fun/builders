#include <iostream>
#include <iomanip>
#include <openssl/md5.h>

int main() {
    const char *data = "hello";
    unsigned char digest[MD5_DIGEST_LENGTH];
    MD5(reinterpret_cast<const unsigned char *>(data), 5, digest);
    for (int i = 0; i < MD5_DIGEST_LENGTH; i++) {
        std::cout << std::hex << std::setfill('0') << std::setw(2) << (int)digest[i];
    }
    std::cout << std::endl;
    return 0;
}
