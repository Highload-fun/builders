using System.Security.Cryptography;
using System.Text;

byte[] hash = MD5.HashData(Encoding.UTF8.GetBytes("hello"));
Console.WriteLine(Convert.ToHexString(hash).ToLower());
