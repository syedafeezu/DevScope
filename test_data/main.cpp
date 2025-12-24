#include <iostream>
#include <string>

class ImageProcessor {
public:
    void process(std::string imagePath) {
        std::cout << "Processing: " << imagePath << std::endl;
    }
};

int main() {
    ImageProcessor processor;
    processor.process("photo.jpg");
    return 0;
}
