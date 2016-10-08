using System;
using System.Threading;
using System.Net.Sockets;
using fastway;

namespace fastway_test
{
	class MainClass
	{
		public static void Main (string[] args)
		{
			var tcpClient = new TcpClient ("127.0.0.1", 10010);
			var netStream = tcpClient.GetStream ();
			var endPoint = new EndPoint (netStream);
			var conn = endPoint.Dial (10086);
			var random = new Random ();

			Thread.Sleep (1000 * 5);

			for (var i = 0; i < 100000; i++) {
				var n = random.Next (1000, 2000);
				var msg1 = new byte[n];

				for (var j = 0; j < n; j++) {
					msg1 [j] = (byte)random.Next (0, 256);
				}

				if (!conn.Send (msg1)) {
					Console.WriteLine ("send failed");
					return;
				}

				byte[] msg2 = null;
				while (msg2 == null) {
					msg2 = conn.Receive ();
					if (msg2 != null && msg2.Length == 0) {
						Console.WriteLine ("msg2.Length == 0");
						return;
					}
				}

				for (var j = 0; j < n; j++) {
					if (msg1 [j] != msg2 [j + 4]) {
						Console.WriteLine ("msg1 [j] != msg2 [j]");
						return;
					}
				}

				Console.WriteLine (i);
			}

			conn.Close ();
			Console.WriteLine ("pass");
		}
	}
}
