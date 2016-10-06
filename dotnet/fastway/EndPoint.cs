using System;
using System.IO;
using System.Threading;
using System.Collections.Generic;

namespace fastway
{
	public delegate void MsgHandler(byte[] msg);
	public delegate void ConnCallback(bool ok, uint connID, uint remoteID);
	public delegate void CloseCallback(uint connID, uint remoteID);

	public class Conn
	{
		public uint ID;
		public uint RemoteID;
	}

	public class EndPoint
	{
		private class ConnCallbacks
		{
			public ConnCallback OnConn;
			public CloseCallback OnClose;

			public ConnCallbacks(ConnCallback onConn, CloseCallback onClose)
			{
				this.OnConn = onConn;
				this.OnClose = onClose;
			}
		}

		private Stream s; // base stream
		private Object l; // lock object
		private Dictionary<uint /* conn id */, Queue<byte[]>> mq; // message queue
		private Dictionary<uint /* remote id */, Queue<ConnCallbacks>> cq; // connect queue
		private Dictionary<uint /* remote id */, Queue<ConnCallbacks>> dq; // dial queue
		private Dictionary<uint /* conn id */, CloseCallback> cc; // close callbacks
		private Dictionary<uint /* conn id */, uint> cr; // conn id to remote id map

		public EndPoint (Stream s)
		{
			this.s = s;
			this.l = new Object ();
			this.mq = new Dictionary<uint, Queue<byte[]>>();
			this.cq = new Dictionary<uint, Queue<ConnCallbacks>> ();
			this.dq = new Dictionary<uint, Queue<ConnCallbacks>> ();
			this.cc = new Dictionary<uint, CloseCallback> ();
			this.cr = new Dictionary<uint, uint> ();

			this.BeginRecvPacket ();
		}

		public void CloseAll()
		{
			this.s.Close ();
		}

		public void Accept(uint remoteID, ConnCallback onConn, CloseCallback onClose)
		{
			lock (this.l) {
				if (!this.cq.ContainsKey(remoteID)) {
					this.cq.Add(remoteID, new Queue<ConnCallbacks>());
				}
				this.cq[remoteID].Enqueue(new ConnCallbacks(onConn, onClose));
			}
		}

		public void Dial(uint remoteID, ConnCallback onConn, CloseCallback onClose) 
		{
			byte[] buf = new byte[13];
			using (MemoryStream ms = new MemoryStream (buf)) {
				using (BinaryWriter bw = new BinaryWriter (ms)) {
					bw.Write ((uint)9);
					bw.Write ((uint)0);
					bw.Write ((byte)0);
					bw.Write (remoteID);
				}
			}
			this.s.BeginWrite (buf, 0, buf.Length, (IAsyncResult result) => {
				this.s.EndWrite(result);
				lock (this.l) {
					if (!this.dq.ContainsKey(remoteID)) {
						this.dq.Add(remoteID, new Queue<ConnCallbacks>());
					}
					this.dq[remoteID].Enqueue(new ConnCallbacks(onConn, onClose));
				}
			}, remoteID);
		}

		public void Process(uint connID, MsgHandler callback)
		{
			lock (this.l) {
				Queue<byte[]> q;
				if (this.mq.TryGetValue (connID, out q)) {
					while (q.Count > 0) {
						callback (q.Dequeue ());
					}
				}
			}
		}

		public void Send(uint connID, byte[] msg)
		{
			byte[] buf = new byte[4 + 4 + msg.Length];
			using (MemoryStream ms = new MemoryStream (buf)) {
				using (BinaryWriter bw = new BinaryWriter (ms)) {
					bw.Write ((uint)4 + msg.Length);
					bw.Write (connID);
					bw.Write (msg);
				}
			}
			this.s.BeginWrite (buf, 0, buf.Length, (IAsyncResult result) => {
				this.s.EndWrite(result);
			}, null);
		}

		public void Close(uint connID, bool invokeCallback)
		{
			lock (this.l) {
				if (this.cc.ContainsKey (connID)) {
					if (invokeCallback) {
						this.cc [connID] (connID, this.cr [connID]);
					}
					this.cc.Remove (connID);
					this.cr.Remove (connID);
					this.mq.Remove (connID);
				}
			}

			byte[] buf = new byte[13];
			using (MemoryStream ms = new MemoryStream (buf)) {
				using (BinaryWriter bw = new BinaryWriter (ms)) {
					bw.Write ((uint)9);
					bw.Write ((uint)0);
					bw.Write ((byte)4);
					bw.Write (connID);
				}
			}
			this.s.BeginWrite (buf, 0, buf.Length, (IAsyncResult result) => {
				this.s.EndWrite(result);
			}, null);
		}

		private void BeginRecvPacket()
		{
			byte[] head = new byte[4];
			this.s.BeginRead (head, 0, 4, (IAsyncResult result1) => {
				byte[] buf = (byte[])result1.AsyncState;
				this.s.EndRead(result1);

				int length;
				using (MemoryStream ms = new MemoryStream (buf)) {
					using (BinaryReader br = new BinaryReader (ms)) {
						length = (int)br.ReadUInt32 ();
					}
				}

				buf = new byte[length];
				this.s.BeginRead (buf, 0, length, (IAsyncResult result2) => {
					byte[] body = (byte[])result2.AsyncState;
					this.s.EndRead(result2);

					uint connID;
					using (MemoryStream ms = new MemoryStream (body)) {
						using (BinaryReader br = new BinaryReader (ms)) {
							connID = br.ReadUInt32 ();
						}
					}

					if (connID != 0) {
						lock (this.l) {
							Queue<byte[]> q;
							if (this.mq.TryGetValue(connID, out q)) {
								q.Enqueue(body);
							} else {
								this.Close(connID, false);
							}
						}
					} else {
						byte cmd = body[4];

						switch (cmd) {
						case 1:
							this.HandleAcceptCmd(body);
							break;
						case 2:
							this.HandleConnectCmd(body);
							break;
						case 3:
							this.HandleRefuseCmd(body);
							break;
						case 4:
							this.HandleCloseCmd(body);
							break;
						case 5:
							this.HandlePingCmd();
							break;
						default:
							throw new Exception("Unsupported Gateway Command");
						}
					}

					this.BeginRecvPacket();
				}, buf);
			}, head);
		}

		private void HandleAcceptCmd(byte[] body)
		{
			uint connID;
			uint remoteID;
			using (MemoryStream ms = new MemoryStream (body, 5, 8)) {
				using (BinaryReader br = new BinaryReader (ms)) {
					connID = br.ReadUInt32 ();
					remoteID = br.ReadUInt32();
				}
			}

			lock (this.l) {
				Queue<ConnCallbacks> q;
				if (this.dq.TryGetValue(remoteID, out q)) {
					ConnCallbacks callbacks = q.Dequeue();
					callbacks.OnConn (true, connID, remoteID);
					this.cc.Add (connID, callbacks.OnClose);
					this.cr.Add (connID, remoteID);
					this.mq.Add (connID, new Queue<byte[]> ());
					return;
				}
			}

			this.Close (connID, false);
		}

		private void HandleConnectCmd(byte[] body)
		{
			uint connID;
			uint remoteID;
			using (MemoryStream ms = new MemoryStream (body, 5, 8)) {
				using (BinaryReader br = new BinaryReader (ms)) {
					connID = br.ReadUInt32 ();
					remoteID = br.ReadUInt32();
				}
			}

			lock (this.l) {
				Queue<ConnCallbacks> q;
				if (this.cq.TryGetValue(remoteID, out q)) {
					ConnCallbacks callbacks = q.Dequeue();
					callbacks.OnConn (true, connID, remoteID);
					this.cc.Add (connID, callbacks.OnClose);
					this.cr.Add (connID, remoteID);
					this.mq.Add (connID, new Queue<byte[]> ());
					return;
				}
			}

			this.Close (connID, false);
		}

		private void HandleRefuseCmd(byte[] body)
		{
			uint remoteID;
			using (MemoryStream ms = new MemoryStream (body, 5, 4)) {
				using (BinaryReader br = new BinaryReader (ms)) {
					remoteID = br.ReadUInt32();
				}
			}

			lock (this.l) {
				Queue<ConnCallbacks> q;
				if (this.dq.TryGetValue(remoteID, out q)) {
					ConnCallbacks callbacks = q.Dequeue();
					callbacks.OnConn(false, 0, remoteID);
				}
			}
		}

		private void HandleCloseCmd(byte[] body)
		{
			uint connID;
			using (MemoryStream ms = new MemoryStream (body, 5, 4)) {
				using (BinaryReader br = new BinaryReader (ms)) {
					connID = br.ReadUInt32();
				}
			}

			lock (this.l) {
				CloseCallback callback;
				if (this.cc.TryGetValue(connID, out callback)) {
					callback (connID, this.cr[connID]);
				}
			}
		}

		private void HandlePingCmd()
		{
			byte[] buf = new byte[8];
			using (MemoryStream ms = new MemoryStream (buf)) {
				using (BinaryWriter bw = new BinaryWriter (ms)) {
					bw.Write ((uint)5);
					bw.Write ((uint)0);
					bw.Write ((byte)5);
				}
			}
			this.s.BeginWrite (buf, 0, buf.Length, (IAsyncResult result) => {
				this.s.EndWrite(result);
			}, null);
		}
	}
}

