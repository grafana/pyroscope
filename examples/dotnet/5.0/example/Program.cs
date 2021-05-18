namespace Example
{
    public class Program
    {
        public static void Main(string[] args)
        {
            while(true) {
                Fast();
                Slow();
            }
        }

        private static void Slow()
        {
            Work(8000);
        }

        private static void Fast()
        {
            Work(2000);
        }

        private static void Work(int n)
        {
            var j = 0;
            for (int i = 0; i < n; i++)
            {
                j++;
            }
        }
    }
}
