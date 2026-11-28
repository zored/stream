using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Threading.Tasks;
using Microsoft.Win32.SafeHandles;

public static class SenseBattery
{
    private const uint ReadAccess = 0x80000000;
    private const uint ShareReadWrite = 3;
    private const uint OpenExisting = 3;
    private const uint Overlapped = 0x40000000;
    private const uint PresentOnly = 0;
    private const uint BufferTooSmall = 0x1a;
    private const int HidSuccess = 0x110000;
    private const int BluetoothReportLength = 78;
    private const int BatteryOffset = 43;
    private const int ReadTimeoutMs = 800;

    [StructLayout(LayoutKind.Sequential)]
    private struct Attributes
    {
        public int Size;
        public ushort Vendor;
        public ushort Product;
        public ushort Version;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct Caps
    {
        public ushort Usage;
        public ushort UsagePage;
        public ushort InputReportByteLength;
        public ushort OutputReportByteLength;
        public ushort FeatureReportByteLength;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 27)]
        public ushort[] RemainingFields;
    }

    [DllImport("hid.dll")]
    private static extern void HidD_GetHidGuid(out Guid guid);

    [DllImport("hid.dll")]
    [return: MarshalAs(UnmanagedType.U1)]
    private static extern bool HidD_GetAttributes(SafeFileHandle handle, ref Attributes attributes);

    [DllImport("hid.dll")]
    [return: MarshalAs(UnmanagedType.U1)]
    private static extern bool HidD_GetPreparsedData(SafeFileHandle handle, out IntPtr data);

    [DllImport("hid.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.U1)]
    private static extern bool HidD_GetFeature(SafeFileHandle handle, [In, Out] byte[] report, int length);

    [DllImport("hid.dll")]
    [return: MarshalAs(UnmanagedType.U1)]
    private static extern bool HidD_FreePreparsedData(IntPtr data);

    [DllImport("hid.dll")]
    private static extern int HidP_GetCaps(IntPtr data, out Caps caps);

    [DllImport("cfgmgr32.dll", CharSet = CharSet.Unicode)]
    private static extern uint CM_Get_Device_Interface_List_SizeW(out uint length, ref Guid guid, string device, uint flags);

    [DllImport("cfgmgr32.dll", CharSet = CharSet.Unicode)]
    private static extern uint CM_Get_Device_Interface_ListW(ref Guid guid, string device, [Out] char[] buffer, uint length, uint flags);

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern SafeFileHandle CreateFileW(string path, uint access, uint share, IntPtr security, uint mode, uint flags, IntPtr template);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern bool CancelIoEx(SafeFileHandle handle, IntPtr overlapped);

    private static string[] Interfaces()
    {
        Guid guid;
        HidD_GetHidGuid(out guid);
        for (int attempt = 0; attempt < 3; attempt++)
        {
            uint length;
            uint result = CM_Get_Device_Interface_List_SizeW(out length, ref guid, null, PresentOnly);
            if (result != 0) throw new IOException("HID interface list size: CONFIGRET " + result);
            var buffer = new char[length];
            result = CM_Get_Device_Interface_ListW(ref guid, null, buffer, length, PresentOnly);
            if (result == 0) return new string(buffer).Split(new[] { '\0' }, StringSplitOptions.RemoveEmptyEntries);
            if (result != BufferTooSmall) throw new IOException("HID interface list: CONFIGRET " + result);
        }
        throw new IOException("HID interfaces changed during enumeration");
    }

    public static Dictionary<int, int> Read()
    {
        var levels = new Dictionary<int, int>();
        foreach (var path in Interfaces())
        {
            var attributes = new Attributes { Size = Marshal.SizeOf(typeof(Attributes)) };
            using (var metadata = CreateFileW(path, 0, ShareReadWrite, IntPtr.Zero, OpenExisting, 0, IntPtr.Zero))
            {
                if (metadata.IsInvalid || !HidD_GetAttributes(metadata, ref attributes)) continue;
            }
            if (attributes.Vendor != 0x054c || (attributes.Product != 0x0e45 && attributes.Product != 0x0e46)) continue;
            if (levels.ContainsKey(attributes.Product)) continue;
            int? level = ReadInterface(path);
            if (level.HasValue) levels.Add(attributes.Product, level.Value);
        }
        return levels;
    }

    private static int? ReadInterface(string path)
    {
        using (var handle = CreateFileW(path, ReadAccess, ShareReadWrite, IntPtr.Zero, OpenExisting, Overlapped, IntPtr.Zero))
        {
            if (handle.IsInvalid)
            {
                int error = Marshal.GetLastWin32Error();
                if (error == 2 || error == 1167) return null;
                throw new Win32Exception(error, "Open Sense HID input");
            }
            IntPtr data;
            if (!HidD_GetPreparsedData(handle, out data)) return null;
            Caps caps;
            try
            {
                if (HidP_GetCaps(data, out caps) != HidSuccess) throw new IOException("Read Sense HID capabilities");
            }
            finally
            {
                HidD_FreePreparsedData(data);
            }
            if (caps.InputReportByteLength < BluetoothReportLength || caps.FeatureReportByteLength == 0) return null;
            var calibration = new byte[caps.FeatureReportByteLength];
            calibration[0] = 0x05;
            HidD_GetFeature(handle, calibration, calibration.Length);

            var report = new byte[caps.InputReportByteLength];
            using (var stream = new FileStream(handle, FileAccess.Read, report.Length, true))
            {
                var clock = Stopwatch.StartNew();
                try
                {
                    while (clock.ElapsedMilliseconds < ReadTimeoutMs)
                    {
                        var read = stream.ReadAsync(report, 0, report.Length);
                        var timeout = Task.Delay(Math.Max(1, ReadTimeoutMs - (int)clock.ElapsedMilliseconds));
                        if (Task.WhenAny(read, timeout).GetAwaiter().GetResult() != read)
                        {
                            CancelIoEx(handle, IntPtr.Zero);
                            return null;
                        }
                        int length = read.GetAwaiter().GetResult();
                        if (length == 0) return null;
                        int? level = ParseBattery(report, length);
                        if (level.HasValue) return level;
                    }
                }
                catch (IOException ex)
                {
                    int error = ex.HResult & 0xffff;
                    if (error != 6 && error != 1167 && error != 995) throw;
                }
            }
        }
        return null;
    }

    private static int? ParseBattery(byte[] report, int length)
    {
        if (length < BluetoothReportLength || report[0] != 0x31) return null;
        int step = report[BatteryOffset] & 0x0f;
        return step <= 10 ? (int?)(step * 10) : null;
    }
}
