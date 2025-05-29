#include <iostream>
#include <fstream>
#include <vector>
#include <cstring>
#include <lzo/lzo1x.h>

void print_usage(const char* program_name) {
    std::cerr << "Usage: " << program_name << " [-c|-d] [--with-size-header] <input_file>" << std::endl;
    std::cerr << "  -c: compress file to stdout" << std::endl;
    std::cerr << "  -d: decompress file to stdout" << std::endl;
    std::cerr << "  --with-size-header: include/expect original size header in compressed data" << std::endl;
}

std::vector<char> read_file(const std::string& filename) {
    std::ifstream file(filename, std::ios::binary);
    if (!file) {
        throw std::runtime_error("Cannot open file: " + filename);
    }

    file.seekg(0, std::ios::end);
    size_t size = file.tellg();
    file.seekg(0, std::ios::beg);

    std::vector<char> buffer(size);
    file.read(buffer.data(), size);
    return buffer;
}

void compress_file(const std::string& filename, bool use_header) {
    std::vector<char> input = read_file(filename);

    // calculate maximum output size
    lzo_uint output_size = input.size() + input.size() / 16 + 64 + 3;
    std::vector<unsigned char> output(output_size);
    std::vector<unsigned char> work_mem(LZO1X_1_MEM_COMPRESS);

    // compress
    int result = lzo1x_1_compress(
        reinterpret_cast<const unsigned char*>(input.data()),
        input.size(),
        output.data(),
        &output_size,
        work_mem.data()
    );

    if (result != LZO_E_OK) {
        throw std::runtime_error("Compression failed");
    }

    if (use_header) {
        // write original size first (8 bytes), then compressed data
        lzo_uint original_size = input.size();
        std::cout.write(reinterpret_cast<const char*>(&original_size), sizeof(original_size));
    }
    std::cout.write(reinterpret_cast<const char*>(output.data()), output_size);
}

void decompress_file(const std::string& filename, bool use_header) {
    std::vector<char> input = read_file(filename);

    if (use_header) {
        // Header mode: read original size from the beginning of the file
        if (input.size() < sizeof(lzo_uint)) {
            throw std::runtime_error("Invalid compressed file format");
        }

        // read original size
        lzo_uint original_size;
        std::memcpy(&original_size, input.data(), sizeof(original_size));

        // prepare output buffer
        std::vector<unsigned char> output(original_size);
        lzo_uint decompressed_size = original_size;

        // decompress (skip the header bytes)
        int result = lzo1x_decompress(
            reinterpret_cast<const unsigned char*>(input.data() + sizeof(lzo_uint)),
            input.size() - sizeof(lzo_uint),
            output.data(),
            &decompressed_size,
            nullptr
        );

        if (result != LZO_E_OK || decompressed_size != original_size) {
            throw std::runtime_error("Decompression failed");
        }

        std::cout.write(reinterpret_cast<const char*>(output.data()), decompressed_size);
    } else {
        // No header mode: we need to guess the output size
        // Start with a buffer that's typically large enough
        lzo_uint estimated_size = input.size() * 3; // reasonable starting estimate
        std::vector<unsigned char> output(estimated_size);
        lzo_uint decompressed_size = estimated_size;

        int result = lzo1x_decompress(
            reinterpret_cast<const unsigned char*>(input.data()),
            input.size(),
            output.data(),
            &decompressed_size,
            nullptr
        );

        if (result == LZO_E_OUTPUT_OVERRUN) {
            // Buffer was too small, try with a larger buffer
            estimated_size = input.size() * 10; // much larger estimate
            output.resize(estimated_size);
            decompressed_size = estimated_size;

            result = lzo1x_decompress(
                reinterpret_cast<const unsigned char*>(input.data()),
                input.size(),
                output.data(),
                &decompressed_size,
                nullptr
            );
        }

        if (result != LZO_E_OK) {
            throw std::runtime_error("Decompression failed");
        }

        std::cout.write(reinterpret_cast<const char*>(output.data()), decompressed_size);
    }
}

int main(int argc, char* argv[]) {
    if (argc < 3 || argc > 4) {
        print_usage(argv[0]);
        return 1;
    }

    // initialize LZO library
    if (lzo_init() != LZO_E_OK) {
        std::cerr << "LZO initialization failed" << std::endl;
        return 1;
    }

    std::string operation;
    std::string filename;
    bool use_header = false;

    // Parse command line arguments
    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];

        if (arg == "-c" || arg == "-d") {
            if (!operation.empty()) {
                std::cerr << "Error: Multiple operation flags specified" << std::endl;
                print_usage(argv[0]);
                return 1;
            }
            operation = arg;
        } else if (arg == "--with-size-header") {
            use_header = true;
        } else {
            if (!filename.empty()) {
                std::cerr << "Error: Multiple filenames specified" << std::endl;
                print_usage(argv[0]);
                return 1;
            }
            filename = arg;
        }
    }

    // Validate arguments
    if (operation.empty()) {
        std::cerr << "Error: No operation specified (-c or -d)" << std::endl;
        print_usage(argv[0]);
        return 1;
    }

    if (filename.empty()) {
        std::cerr << "Error: No input file specified" << std::endl;
        print_usage(argv[0]);
        return 1;
    }

    try {
        if (operation == "-c") {
            compress_file(filename, use_header);
        } else if (operation == "-d") {
            decompress_file(filename, use_header);
        }
    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
}