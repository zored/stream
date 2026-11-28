using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using System.Threading.Tasks;
using Microsoft.Win32.SafeHandles;

public static class SteamBattery
{
    private const int BatteryReportLength = 15;
    private const int ReadTimeoutMs = 5000;
    private const uint ReadAccess = 0x80000000;
    private const uint ShareReadWrite = 3;
    private const uint OpenExisting = 3;
    private const uint Overlapped = 0x40000000;

    [StructLayout(LayoutKind.Sequential)]
    private struct Caps
    {
        public ushort Usage;
        public ushort UsagePage;
        public ushort InputLength;
        [MarshalAs(UnmanagedType.ByValArray, SizeConst = 29)]
        public ushort[] RemainingFields;
    }

    [DllImport("hid.dll")]
    private static extern void HidD_GetHidGuid(out Guid guid);

    [DllImport("hid.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.U1)]
    private static extern bool HidD_GetPreparsedData(SafeFileHandle handle, out IntPtr data);

    [DllImport("hid.dll")]
    [return: MarshalAs(UnmanagedType.U1)]
    private static extern bool HidD_FreePreparsedData(IntPtr data);

    [DllImport("hid.dll")]
    private static extern int HidP_GetCaps(IntPtr data, out Caps caps);

    [DllImport("cfgmgr32.dll", CharSet = CharSet.Unicode, ExactSpelling = true)]
    private static extern uint CM_Get_Device_Interface_List_SizeW(out uint size, ref Guid guid, string device, uint flags);

    [DllImport("cfgmgr32.dll", CharSet = CharSet.Unicode, ExactSpelling = true)]
    private static extern uint CM_Get_Device_Interface_ListW(ref Guid guid, string device, [Out] char[] buffer, uint size, uint flags);

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern SafeFileHandle CreateFileW(string path, uint access, uint share, IntPtr security, uint mode, uint flags, IntPtr template);

    [DllImport("kernel32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CancelIoEx(SafeFileHandle handle, IntPtr overlapped);

    public static int? Read()
    {
        var reads = new List<Task<int?>>();
        var paths = Interfaces();
        Array.Sort(paths, StringComparer.OrdinalIgnoreCase);
        foreach (var path in paths)
        {
            if (path.IndexOf("vid_28de&pid_1302", StringComparison.OrdinalIgnoreCase) >= 0 ||
                path.IndexOf("vid_28de&pid_1304", StringComparison.OrdinalIgnoreCase) >= 0 ||
                path.IndexOf("vid_28de&pid_1305", StringComparison.OrdinalIgnoreCase) >= 0)
            {
                reads.Add(ReadInterface(path));
            }
        }

        foreach (int? percent in Task.WhenAll(reads).GetAwaiter().GetResult())
        {
            if (percent.HasValue) return percent;
        }
        return null;
    }

    private static string[] Interfaces()
    {
        Guid guid;
        HidD_GetHidGuid(out guid);
        for (int attempt = 0; attempt < 3; attempt++)
        {
            uint size;
            uint result = CM_Get_Device_Interface_List_SizeW(out size, ref guid, null, 0);
            if (result != 0) throw new IOException("Steam HID interface list size: CONFIGRET " + result);
            var buffer = new char[size];
            result = CM_Get_Device_Interface_ListW(ref guid, null, buffer, size, 0);
            if (result == 0) return new string(buffer).Split(new[] { '\0' }, StringSplitOptions.RemoveEmptyEntries);
            if (result != 0x1a) throw new IOException("Steam HID interface list: CONFIGRET " + result);
        }
        throw new IOException("Steam HID interfaces changed during enumeration");
    }

    private static async Task<int?> ReadInterface(string path)
    {
        using (var metadata = CreateFileW(path, 0, ShareReadWrite, IntPtr.Zero, OpenExisting, 0, IntPtr.Zero))
        {
            if (metadata.IsInvalid) return Unavailable(Marshal.GetLastWin32Error(), "Open Steam HID metadata");
            IntPtr data;
            if (!HidD_GetPreparsedData(metadata, out data))
                return Unavailable(Marshal.GetLastWin32Error(), "Read Steam HID metadata");
            Caps caps;
            try
            {
                if (HidP_GetCaps(data, out caps) != 0x110000) throw new IOException("Read Steam HID capabilities");
            }
            finally
            {
                HidD_FreePreparsedData(data);
            }
            if (caps.UsagePage != 0xff00 || caps.Usage != 1 || caps.InputLength < BatteryReportLength) return null;

            using (var handle = CreateFileW(path, ReadAccess, ShareReadWrite, IntPtr.Zero, OpenExisting, Overlapped, IntPtr.Zero))
            {
                if (handle.IsInvalid) return Unavailable(Marshal.GetLastWin32Error(), "Open Steam HID input");
                var report = new byte[caps.InputLength];
                using (var stream = new FileStream(handle, FileAccess.Read, report.Length, true))
                {
                    var timeout = Task.Delay(ReadTimeoutMs);
                    try
                    {
                        while (!timeout.IsCompleted)
                        {
                            var read = stream.ReadAsync(report, 0, report.Length);
                            if (await Task.WhenAny(read, timeout) != read)
                            {
                                CancelIoEx(handle, IntPtr.Zero);
                                await read;
                                return null;
                            }
                            int length = await read;
                            if (length == 0) return null;
                            int? percent = ParseBattery(report, length);
                            if (percent.HasValue) return percent;
                        }
                    }
                    catch (OperationCanceledException)
                    {
                        return null;
                    }
                    catch (IOException ex)
                    {
                        if (!IsUnavailable(ex.HResult & 0xffff)) throw;
                        return null;
                    }
                }
            }
        }
        return null;
    }

    private static int? ParseBattery(byte[] report, int length)
    {
        if (length < BatteryReportLength || report[0] != 0x43) return null;
        if (report[1] != 1 && report[1] != 2 && report[1] != 4) return null;
        return report[2] <= 100 ? (int?)report[2] : null;
    }

    private static int? Unavailable(int error, string operation)
    {
        if (IsUnavailable(error)) return null;
        throw new Win32Exception(error, operation);
    }

    private static bool IsUnavailable(int error)
    {
        return error == 2 || error == 6 || error == 995 || error == 1167;
    }
}
