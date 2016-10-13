using System;
using System.Threading;
using System.Net.Sockets;
using Fastway;

namespace FastwayTest
{
	class MainClass
	{
		public static void Main (string[] args)
		{
			var tcpClient = new TcpClient ("127.0.0.1", 10010);
			var netStream = tcpClient.GetStream ();
			var endPoint = new EndPoint (netStream, 1000, 0, null);
			var conn = endPoint.Dial (10086);
			var random = new Random ();

			Thread.Sleep (1000 * 5);

			for (var i = 0; i < 100000; i++) {
				var n = random.Next (10, 2000);
				var msg1 = new byte[n];
				random.NextBytes(msg1);

				if (!conn.Send (msg1)) {
					Console.WriteLine ("send failed");
					return;
				}

				byte[] msg2 = null;
				for (;;) {
					msg2 = conn.Receive ();
					if (msg2 == null) {
						Console.WriteLine ("msg2.Length == 0");
						return;
					}
					if (msg2 == Conn.NoMsg) {
						continue;
					}
					break;
				}

				if (msg1.Length != msg2.Length) {
					Console.WriteLine ("msg1.Length != msg2.Length, {0}, {1}", msg1.Length, msg2.Length);
					return;
				}

				for (var j = 0; j < n; j++) {
					if (msg1 [j] != msg2 [j]) {
						Console.WriteLine ("msg1 [j] != msg2 [j]");
						return;
					}
				}

				Console.WriteLine ("{0}, {1}", i, msg1.Length);
			}

			conn.Close ();
			Console.WriteLine ("pass");
		}
	}
}
