namespace Example
{
    public class Program
    {
        public static void Main(string[] args)
        {
            while (true)
            {
                _ = new Fast.Work();
                _ = new Slow.Work();
            }
        }
    }
}

namespace Slow
{
    public class Work
    {
        public Work()
        {
            var j = 0;
            for (var i = 0; i < 8000; i++) j++;
        }
    }
}

namespace Fast
{
    public class Work
    {
        public Work()
        {
            var j = 0;
            for (var i = 0; i < 2000; i++) j++;
        }
    }
}
