const std = @import("std");

pub fn main() void {
    var h = std.crypto.hash.Md5.init(.{});
    h.update("hello");
    var digest: [16]u8 = undefined;
    h.final(&digest);
    const stdout = std.io.getStdOut().writer();
    for (digest) |byte| {
        stdout.print("{x:0>2}", .{byte}) catch return;
    }
    stdout.print("\n", .{}) catch return;
}
